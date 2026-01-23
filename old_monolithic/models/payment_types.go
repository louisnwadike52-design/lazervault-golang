package models

// PaymentMethodType represents the type of payment method
type PaymentMethodType string

const (
	PaymentMethodTypeUnspecified    PaymentMethodType = "PAYMENT_METHOD_UNSPECIFIED"
	PaymentMethodTypeCreditCard     PaymentMethodType = "PAYMENT_METHOD_CREDIT_CARD"
	PaymentMethodTypeDebitCard      PaymentMethodType = "PAYMENT_METHOD_DEBIT_CARD"
	PaymentMethodTypeBankTransfer   PaymentMethodType = "PAYMENT_METHOD_BANK_TRANSFER"
	PaymentMethodTypeAccountBalance PaymentMethodType = "PAYMENT_METHOD_ACCOUNT_BALANCE"
	PaymentMethodTypePayPal         PaymentMethodType = "PAYMENT_METHOD_PAYPAL"
	PaymentMethodTypeApplePay       PaymentMethodType = "PAYMENT_METHOD_APPLE_PAY"
	PaymentMethodTypeGooglePay      PaymentMethodType = "PAYMENT_METHOD_GOOGLE_PAY"
	PaymentMethodTypeBitcoin        PaymentMethodType = "PAYMENT_METHOD_BITCOIN"
	PaymentMethodTypeEthereum       PaymentMethodType = "PAYMENT_METHOD_ETHEREUM"
	PaymentMethodTypeUSDC           PaymentMethodType = "PAYMENT_METHOD_USDC"
)

// InvoicePaymentStatus represents the status of an invoice payment
type InvoicePaymentStatus string

const (
	InvoicePaymentStatusPending       InvoicePaymentStatus = "pending"
	InvoicePaymentStatusProcessing    InvoicePaymentStatus = "processing"
	InvoicePaymentStatusCompleted     InvoicePaymentStatus = "completed"
	InvoicePaymentStatusFailed        InvoicePaymentStatus = "failed"
	InvoicePaymentStatusCancelled     InvoicePaymentStatus = "cancelled"
	InvoicePaymentStatusPartiallyPaid InvoicePaymentStatus = "partially_paid"
	InvoicePaymentStatusOverdue       InvoicePaymentStatus = "overdue"
	InvoicePaymentStatusRefunded      InvoicePaymentStatus = "refunded"
	InvoicePaymentStatusDisputed      InvoicePaymentStatus = "disputed"
)

// DisputeStatus represents the status of a payment dispute
type DisputeStatus string

const (
	DisputeStatusPending       DisputeStatus = "pending"
	DisputeStatusInvestigating DisputeStatus = "investigating"
	DisputeStatusResolved      DisputeStatus = "resolved"
	DisputeStatusRejected      DisputeStatus = "rejected"
	DisputeStatusEscalated     DisputeStatus = "escalated"
)
