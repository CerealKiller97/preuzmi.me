package dto

type (
	Bill struct {
		// TotalAmountFormatted is always present in the raw API response
		// ("1.819,46"). TotalAmount is the numeric value some response variants
		// also include; it may be absent (0), in which case the formatted string
		// is parsed instead.
		TotalAmountFormatted     string  `json:"totalAmountFormatted"`
		TotalDebtAmountFormatted string  `json:"totalDebtAmountFormatted"`
		Status                   Status  `json:"status"`
		InvoiceNumber            string  `json:"invoiceNumber"`
		BillingAccountID         string  `json:"billingAccountId"`
		TotalAmount              float64 `json:"totalAmount"`
		// Month is 0-indexed as MTS returns it (0 = January … 11 = December);
		// the July bill arrives as 6. Convert to 1-12 before formatting a period.
		Month              int  `json:"month"`
		Year               int  `json:"year"`
		ComplaintAvailable bool `json:"complaintAvailable"`
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

	Status string
)

const (
	NotPaid Status = "NOT_PAID"
	Paid    Status = "PAID"
)
