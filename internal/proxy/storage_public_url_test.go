package proxy

import "testing"

// Whose URL is it, and may we rewrite its host?
//
// toPublicURL exists for the LOCAL storage backend, whose URLs point at an
// internal address a phone cannot reach. A CLOUD backend returns a presigned
// URL whose signature covers the host — rewriting it both breaks the signature
// and aims the request at an origin that does not serve the path.
//
// Production, 2026-10-01, STORAGE_PROVIDER=r2: every upload in the app failed
// with HTTP 404 because the R2 presigned host was being replaced with
// api.lazervault.app. FCY documents, profile pictures, bank scans, chat media
// (images and voice notes), invoice attachments, escrow evidence — all of it.
func newProxyForTest(storageBase, publicBase string) *StorageProxy {
	return &StorageProxy{
		storageBaseURL: storageBase,
		publicBaseURL:  publicBase,
	}
}

func TestPresignedCloudURLsAreLeftAlone(t *testing.T) {
	p := newProxyForTest("http://127.0.0.1:18094", "https://api.lazervault.app")

	for _, u := range []string{
		// R2, virtual-hosted style — the exact shape production returns.
		"https://lazervault-media.420941192248aadb04c188bdf1640056.r2.cloudflarestorage.com/users/u1/fcy-documents/a.pdf?X-Amz-Signature=abc",
		// S3 and GCS equivalents.
		"https://lazervault-media.s3.eu-west-2.amazonaws.com/users/u1/chat-media/a.jpg?X-Amz-Signature=abc",
		"https://storage.googleapis.com/lazervault-media/users/u1/profile-a.png?X-Goog-Signature=abc",
	} {
		if got := p.toPublicURL(u); got != u {
			t.Errorf("presigned URL was rewritten\n  in:  %s\n  out: %s", u, got)
		}
	}
}

func TestLocalBackendURLsAreRehosted(t *testing.T) {
	p := newProxyForTest("http://127.0.0.1:18094", "https://api.lazervault.app")

	cases := map[string]string{
		// By path — the storage service's own object route.
		"http://127.0.0.1:18094/v1/storage/objects/users/u1/a.pdf?sig=x": "https://api.lazervault.app/v1/storage/objects/users/u1/a.pdf?sig=x",
		// By host — the upstream we proxy to, even on a route we don't recognise.
		"http://127.0.0.1:18094/something/else": "https://api.lazervault.app/something/else",
		// Loopback spelled differently.
		"http://localhost:18094/v1/storage/objects/users/u1/b.png": "https://api.lazervault.app/v1/storage/objects/users/u1/b.png",
	}
	for in, want := range cases {
		if got := p.toPublicURL(in); got != want {
			t.Errorf("local URL not re-hosted\n  in:   %s\n  got:  %s\n  want: %s", in, got, want)
		}
	}
}

func TestRewriteIsSkippedWhenUnconfigured(t *testing.T) {
	// No public base: nothing to rewrite to, so the URL passes through rather
	// than being mangled into a host-less string.
	p := newProxyForTest("http://127.0.0.1:18094", "")
	const u = "http://127.0.0.1:18094/v1/storage/objects/users/u1/a.pdf"
	if got := p.toPublicURL(u); got != u {
		t.Errorf("got %s, want %s", got, u)
	}
	if got := p.toPublicURL(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

func TestAStorageHostnameIsMatchedNotGuessed(t *testing.T) {
	// A deployment where storage is a named internal host, not loopback.
	p := newProxyForTest("http://storage-internal:18094", "https://api.lazervault.app")

	if got := p.toPublicURL("http://storage-internal:18094/v1/storage/objects/users/u1/a.pdf"); got !=
		"https://api.lazervault.app/v1/storage/objects/users/u1/a.pdf" {
		t.Errorf("the configured storage host must be re-hosted, got %s", got)
	}
	// A lookalike must NOT match: substring matching on hostnames is how an
	// attacker-controlled origin gets treated as ours.
	const foreign = "https://storage-internal.evil.example/users/u1/a.pdf?X-Amz-Signature=abc"
	if got := p.toPublicURL(foreign); got != foreign {
		t.Errorf("a lookalike host must be left alone, got %s", got)
	}
}
