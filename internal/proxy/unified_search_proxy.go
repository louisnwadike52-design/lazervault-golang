package proxy

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	accountspb "accounts-service/proto"
	groupaccountspb "group-accounts-service/proto"
	pb "lazervaultGo/pb"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

// UnifiedSearchProxy composes a LOCAL-first → GLOBAL user search as a single
// BFF aggregation over two EXISTING gRPC APIs (no new microservice RPC):
//
//  1. LOCAL: the caller's own SAVED RECIPIENTS (accounts-service ListRecipients),
//     matched in-gateway on alias, name, email, phone, account number — with
//     ALIAS matches ranked first. This is what lets "send 500 to Mum" resolve
//     a user-defined nickname before hitting the global directory.
//  2. GLOBAL: the platform user directory (auth-service SearchUsers: username,
//     name, email, phone), paginated, de-duplicated against the local matches.
//  3. ORG: users that belong to organization/group accounts
//     (group-accounts-service SearchUsers). These would otherwise never appear
//     in the directory search, so an org-only member was previously
//     un-findable. Deduped against local + global + self, appended to global.
//
// A single user's saved-recipient set is small and indexed by user_id, so we
// pull it once (bounded by localRecipientCap) and filter in memory — cheap, no
// N+1, no proto churn. Reusable by Flutter, chat, and voice over HTTP.
type UnifiedSearchProxy struct {
	recipientClient accountspb.RecipientServiceClient
	authClient      pb.AuthServiceClient
	// groupAcctClient is optional — when nil (dial failed at boot) org users are
	// simply omitted and the local + global search still works.
	groupAcctClient groupaccountspb.GroupAccountServiceClient
}

// NewUnifiedSearchProxy builds the proxy from the same gRPC clients the
// recipient/auth proxies already use. groupAcctClient may be nil (org search
// then degrades gracefully).
func NewUnifiedSearchProxy(rc accountspb.RecipientServiceClient, ac pb.AuthServiceClient, gac groupaccountspb.GroupAccountServiceClient) *UnifiedSearchProxy {
	return &UnifiedSearchProxy{recipientClient: rc, authClient: ac, groupAcctClient: gac}
}

// localRecipientCap bounds the saved-recipient set we pull for in-gateway
// matching. A single user's saved list is small; 500 covers essentially
// everyone while keeping the call cheap.
const localRecipientCap = 500

// minQueryRunes mirrors auth-service's minimum query length (2) so we never
// fan out an unbounded search on a single character.
const minQueryRunes = 2

// unifiedResultItem is the merged shape returned for both saved and global hits.
type unifiedResultItem struct {
	Source           string `json:"source"`       // "saved" | "global"
	UserID           string `json:"user_id"`      // internal user UUID (empty for external saved recipients)
	RecipientID      string `json:"recipient_id"` // saved recipient id (saved only)
	DisplayName      string `json:"display_name"` // alias if present, else name/username
	Name             string `json:"name"`
	Alias            string `json:"alias"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	PhoneNumber      string `json:"phone_number"`
	AccountNumber    string `json:"account_number"`
	BankName         string `json:"bank_name"`
	ProfilePicture   string `json:"profile_picture"`
	PrimaryAccountID string `json:"primary_account_id"`
	IsLazervault     bool   `json:"is_lazervault_user"`
	IsSaved          bool   `json:"is_saved"`
	IsFavorite       bool   `json:"is_favorite"`
	MatchedField     string `json:"matched_field"` // alias|name|email|phone|account|username|""
	Type             string `json:"type"`          // internal|external
}

// HandleUnifiedSearch serves GET /api/v1/users/search-unified?q=&limit=&offset=
//
// Response: {local:[...], global:[...], has_more:bool, next_offset:int}.
// `local` is populated only on the first page (offset==0); subsequent pages
// continue paginating the global directory.
func (p *UnifiedSearchProxy) HandleUnifiedSearch(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		query = strings.TrimSpace(c.Query("query"))
	}
	limit := atoiDefault(c.Query("limit"), 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := atoiDefault(c.Query("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	if len([]rune(query)) < minQueryRunes {
		c.JSON(http.StatusOK, gin.H{
			"local": []unifiedResultItem{}, "global": []unifiedResultItem{},
			"has_more": false, "next_offset": 0,
		})
		return
	}

	// Forward the caller's bearer token to downstream gRPC so both services
	// authenticate as the SAME user (local recipients are per-user; global
	// search runs in the caller's auth context).
	outCtx := p.outgoingCtx(c)

	// The authenticated caller — NEVER return the user to themselves (you can't
	// send to / tag / split with yourself). Set by JWTAuthMiddleware.
	callerID := strings.TrimSpace(c.GetString("user_id"))

	// TEMP diagnostic: confirm auth is reaching the downstream calls.
	log.Info().
		Str("q", query).
		Bool("has_auth_header", c.GetHeader("Authorization") != "").
		Bool("has_caller_id", callerID != "").
		Int("offset", offset).
		Msg("unified-search: handling request")

	q := strings.ToLower(query)
	digits := digitsOnly(query)

	// ---- Phase 1: LOCAL saved recipients (first page only) ----
	local := []unifiedResultItem{}
	localUserIDs := map[string]bool{}
	if offset == 0 {
		page, pageSize := int32(1), int32(localRecipientCap)
		lr, lerr := p.recipientClient.ListRecipients(outCtx, &accountspb.ListRecipientsRequest{
			Page: &page, PageSize: &pageSize,
		})
		if lerr != nil {
			log.Error().Err(lerr).Str("q", query).Msg("unified-search: ListRecipients (local) failed")
		}
		if lerr == nil && lr != nil {
			for _, r := range lr.GetRecipients() {
				if callerID != "" && r.GetInternalUserId() == callerID {
					continue // never the caller themselves
				}
				field := matchRecipient(r, q, digits)
				if field == "" {
					continue
				}
				local = append(local, recipientToItem(r, field))
				if uid := r.GetInternalUserId(); uid != "" {
					localUserIDs[uid] = true
				}
			}
			// alias matches first, then favorites, then name A→Z.
			sort.SliceStable(local, func(i, j int) bool {
				ai, aj := local[i].MatchedField == "alias", local[j].MatchedField == "alias"
				if ai != aj {
					return ai
				}
				if local[i].IsFavorite != local[j].IsFavorite {
					return local[i].IsFavorite
				}
				return strings.ToLower(local[i].DisplayName) < strings.ToLower(local[j].DisplayName)
			})
			// Saved INTERNAL recipients don't store a username, but username-keyed
			// surfaces (split-bill, tag-pay, family invite) reject empty usernames
			// as "not on LazerVault". Resolve the handle from the directory so a
			// saved LazerVault contact picked by alias is fully usable.
			p.enrichUsernames(outCtx, local)
		}
	}

	// ---- Phase 2: DIRECTORY users (auth-service, paginated, deduped vs local+self)
	directory := []unifiedResultItem{}
	hasMore := false
	gr, gerr := p.authClient.SearchUsers(outCtx, &pb.UserSearchRequest{
		Query: query, Limit: int32(limit), Offset: int32(offset),
	})
	if gerr != nil {
		log.Error().Err(gerr).Str("q", query).Msg("unified-search: SearchUsers (global) failed")
	}
	if gerr == nil && gr != nil {
		raw := gr.GetUsers()
		hasMore = len(raw) >= limit // full page back ⇒ probably more
		for _, u := range raw {
			if callerID != "" && u.GetUserId() == callerID {
				continue // never the caller themselves
			}
			if localUserIDs[u.GetUserId()] {
				continue // already shown as a saved contact above
			}
			directory = append(directory, userToItem(u))
		}
	}

	// ---- Phase 3: FINANCIAL CONNECTIONS (org/group members) ----
	// Ranked ABOVE the general directory — a group/org connection is a stronger
	// relationship than a stranger, so the search order is:
	//   saved recipients (alias-first) → financial connections → general users.
	// FIRST PAGE ONLY: org search isn't paginated, so fetching it per-page would
	// duplicate members across load-more pages. Best-effort: a failure here must
	// never blank the whole result.
	orgItems := []unifiedResultItem{}
	orgSeen := make(map[string]bool)
	if offset == 0 && p.groupAcctClient != nil {
		if or, oerr := p.groupAcctClient.SearchUsers(outCtx, &groupaccountspb.SearchUsersRequest{
			Query: query, Limit: int32(limit),
		}); oerr != nil {
			log.Error().Err(oerr).Str("q", query).Msg("unified-search: SearchUsers (org) failed")
		} else if or != nil {
			for _, m := range or.GetUsers() {
				uid := m.GetUserId()
				if uid == "" || orgSeen[uid] {
					continue
				}
				if callerID != "" && uid == callerID {
					continue // never the caller themselves
				}
				if localUserIDs[uid] {
					continue // already shown as a saved contact above
				}
				orgSeen[uid] = true
				orgItems = append(orgItems, groupMemberToItem(m))
			}
		}
	}

	// Assemble global = financial connections FIRST, then the general directory
	// with anyone already shown as a connection removed (the connection wins).
	global := make([]unifiedResultItem, 0, len(orgItems)+len(directory))
	global = append(global, orgItems...)
	for _, d := range directory {
		if d.UserID != "" && orgSeen[d.UserID] {
			continue
		}
		global = append(global, d)
	}

	// Don't mask a real failure as "no matches": if the directory search errored
	// and we have nothing local to fall back on, surface it so the app shows an
	// error state (and the cause lands in the gateway log above) instead of an
	// empty, misleading 200.
	if gerr != nil && len(local) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "search_unavailable",
			"message": "Search is temporarily unavailable. Please try again.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"local":       local,
		"global":      global,
		"has_more":    hasMore,
		"next_offset": offset + limit,
	})
}

// enrichUsernames fills in the LazerVault @handle for saved INTERNAL recipients
// that don't carry one (the recipient row stores no username). It resolves each
// by searching the directory for the recipient's name and matching the internal
// user id. Bounded to maxEnrich lookups so an alias query can't fan out.
func (p *UnifiedSearchProxy) enrichUsernames(ctx context.Context, items []unifiedResultItem) {
	const maxEnrich = 12
	done := 0
	for i := range items {
		if done >= maxEnrich {
			return
		}
		it := &items[i]
		if it.Source != "saved" || !it.IsLazervault || it.UserID == "" ||
			it.Username != "" || strings.TrimSpace(it.Name) == "" {
			continue
		}
		done++
		gr, err := p.authClient.SearchUsers(ctx, &pb.UserSearchRequest{
			Query: it.Name, Limit: 10,
		})
		if err != nil || gr == nil {
			continue
		}
		for _, u := range gr.GetUsers() {
			if u.GetUserId() == it.UserID {
				it.Username = u.GetUsername()
				if it.ProfilePicture == "" {
					it.ProfilePicture = u.GetProfilePicture()
				}
				if it.PrimaryAccountID == "" {
					it.PrimaryAccountID = u.GetPrimaryAccountId()
				}
				break
			}
		}
	}
}

// outgoingCtx builds a downstream gRPC context carrying the caller's auth.
func (p *UnifiedSearchProxy) outgoingCtx(c *gin.Context) context.Context {
	md := metadata.New(map[string]string{})
	if auth := c.GetHeader("Authorization"); auth != "" {
		md.Set("authorization", auth)
	}
	// CRITICAL: downstream services (auth-service.SearchUsers,
	// accounts-service.ListRecipients) authenticate via the x-user-id METADATA,
	// not by parsing the JWT. This BFF calls them directly over gRPC, bypassing
	// grpc-gateway's incoming-header→metadata mapping that normally injects
	// x-user-id — so we must set it explicitly from the JWT-verified caller
	// (JWTAuthMiddleware put it in the gin context). Without this,
	// extractUserIDFromContext fails, SearchUsers returns "Authentication
	// required" with an empty list, and the WHOLE unified search silently
	// returns nothing.
	if uid := strings.TrimSpace(c.GetString("user_id")); uid != "" {
		md.Set("x-user-id", uid)
	}
	if rid := c.GetHeader("X-Request-Id"); rid != "" {
		md.Set("x-request-id", rid)
	}
	return metadata.NewOutgoingContext(c.Request.Context(), md)
}

// matchRecipient returns the first field the query matches ("" = no match).
// Order encodes priority: alias > name > email > phone > account.
func matchRecipient(r *accountspb.Recipient, q, digits string) string {
	if a := strings.ToLower(strings.TrimSpace(r.GetAlias())); a != "" && strings.Contains(a, q) {
		return "alias"
	}
	if strings.Contains(strings.ToLower(r.GetName()), q) {
		return "name"
	}
	if e := strings.ToLower(r.GetEmail()); e != "" && strings.Contains(e, q) {
		return "email"
	}
	if digits != "" {
		if pn := digitsOnly(r.GetPhoneNumber()); pn != "" && strings.Contains(pn, digits) {
			return "phone"
		}
		if an := digitsOnly(r.GetAccountNumber()); an != "" && strings.Contains(an, digits) {
			return "account"
		}
	}
	return ""
}

func recipientToItem(r *accountspb.Recipient, field string) unifiedResultItem {
	display := r.GetName()
	if a := strings.TrimSpace(r.GetAlias()); a != "" {
		display = a
	}
	return unifiedResultItem{
		Source:        "saved",
		UserID:        r.GetInternalUserId(),
		RecipientID:   strconv.FormatUint(r.GetId(), 10),
		DisplayName:   display,
		Name:          r.GetName(),
		Alias:         r.GetAlias(),
		Email:         r.GetEmail(),
		PhoneNumber:   r.GetPhoneNumber(),
		AccountNumber: r.GetAccountNumber(),
		BankName:      r.GetBankName(),
		IsLazervault:  r.GetType() == "internal",
		IsSaved:       true,
		IsFavorite:    r.GetIsFavorite(),
		MatchedField:  field,
		Type:          r.GetType(),
	}
}

func userToItem(u *pb.UserLookupResult) unifiedResultItem {
	name := strings.TrimSpace(u.GetFirstName() + " " + u.GetLastName())
	display := name
	if display == "" {
		display = u.GetUsername()
	}
	return unifiedResultItem{
		Source:           "global",
		UserID:           u.GetUserId(),
		DisplayName:      display,
		Name:             name,
		Username:         u.GetUsername(),
		Email:            u.GetEmail(),
		PhoneNumber:      u.GetPhoneNumber(),
		ProfilePicture:   u.GetProfilePicture(),
		PrimaryAccountID: u.GetPrimaryAccountId(),
		IsLazervault:     u.GetIsLazervaultUser(),
		Type:             "internal",
	}
}

// groupMemberToItem maps a group-accounts-service member into the unified result
// shape (org users are internal Lazervault users found via their org membership).
func groupMemberToItem(m *groupaccountspb.GroupMemberMessage) unifiedResultItem {
	name := strings.TrimSpace(m.GetUserName())
	display := name
	if display == "" {
		display = m.GetUserUsername()
	}
	return unifiedResultItem{
		Source:         "global",
		UserID:         m.GetUserId(),
		DisplayName:    display,
		Name:           name,
		Username:       m.GetUserUsername(),
		Email:          m.GetEmail(),
		PhoneNumber:    m.GetPhoneNumber(),
		ProfilePicture: m.GetProfileImage(),
		IsLazervault:   true,
		Type:           "internal",
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

// digitsOnly strips every non-digit rune (for phone/account substring match).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}
