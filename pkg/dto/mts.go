package dto

type (
	Bill struct {
		IsPayable                bool                     `json:"isPayable"`
		TotalAmountFormatted     string                   `json:"totalAmountFormatted"`
		TotalDebtAmountFormatted TotalDebtAmountFormatted `json:"totalDebtAmountFormatted"`
		Month                    int64                    `json:"month"`
		Year                     int64                    `json:"year"`
		Status                   Status                   `json:"status"`
		InvoiceNumber            string                   `json:"invoiceNumber"`
		BillingAccountID         string                   `json:"billingAccountId"`
		ComplaintAvailable       bool                     `json:"complaintAvailable"`
	}
	BillGroup struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Bills       []Bill `json:"bills"`
	}

	GetReceiptsResponse struct {
		BillGroups  []BillGroup `json:"billGroups"`
		Count       int64       `json:"count"`
		Description string      `json:"description"`
		MaxLimit    int64       `json:"maxLimit"`
	}

	Status                   string
	TotalDebtAmountFormatted string
)

const (
	NotPaid Status = "NOT_PAID"
	Paid    Status = "PAID"
)
