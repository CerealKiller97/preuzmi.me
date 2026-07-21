package dto

type (
	Bill struct {
		TotalAmountFormatted     string                   `json:"totalAmountFormatted"`
		TotalDebtAmountFormatted TotalDebtAmountFormatted `json:"totalDebtAmountFormatted"`
		Status                   Status                   `json:"status"`
		InvoiceNumber            string                   `json:"invoiceNumber"`
		BillingAccountID         string                   `json:"billingAccountId"`
		Month                    int64                    `json:"month"`
		Year                     int64                    `json:"year"`
		IsPayable                bool                     `json:"isPayable"`
		ComplaintAvailable       bool                     `json:"complaintAvailable"`
	}
	BillGroup struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Bills       []Bill `json:"bills"`
	}

	GetReceiptsResponse struct {
		Description string      `json:"description"`
		BillGroups  []BillGroup `json:"billGroups"`
		Count       int64       `json:"count"`
		MaxLimit    int64       `json:"maxLimit"`
	}

	Status                   string
	TotalDebtAmountFormatted string
)

const (
	NotPaid Status = "NOT_PAID"
	Paid    Status = "PAID"
)
