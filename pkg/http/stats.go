package http

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/rs/zerolog/log"
)

type ProviderTotal struct {
	Provider string  `json:"provider"`
	Amount   float64 `json:"amount"`
}

// ProviderMonthly carries one provider's amounts for every month of the year,
// so the UI can draw a separate chart line and table column per provider.
type ProviderMonthly struct {
	Provider string    `json:"provider"`
	Monthly  []float64 `json:"monthly"`
}

type StatsResponse struct {
	Currency          string            `json:"currency"`
	Monthly           []float64         `json:"monthly"`
	MonthlyCounts     []int             `json:"monthly_counts"`
	ByProvider        []ProviderTotal   `json:"by_provider"`
	MonthlyByProvider []ProviderMonthly `json:"monthly_by_provider"`
	Year              int               `json:"year"`
	Total             float64           `json:"total"`
	Average           float64           `json:"average"`
}

// statsAPIHandler aggregates monthly and annual totals; query: year (int), provider (comma-separated)
func statsAPIHandler(cfg *config.Config, receiptsStore func() *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		// Amounts come from the receipts database (price), joined onto the
		// receipts by collectReceipts.
		items, err := collectReceipts(cfg, nil, receiptsStore())
		if err != nil {
			log.Err(err).Msg("Error scanning receipts for stats")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Provider filter
		allowed := map[string]struct{}{}
		providersParam := strings.TrimSpace(r.URL.Query().Get("provider"))
		if providersParam != "" {
			for _, p := range strings.Split(providersParam, ",") {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" {
					allowed[p] = struct{}{}
				}
			}
		}
		// Year
		year := 0
		if yStr := strings.TrimSpace(r.URL.Query().Get("year")); yStr != "" {
			if y, err := strconv.Atoi(yStr); err == nil {
				year = y
			}
		}
		if year == 0 {
			year = time.Now().Year()
		}
		// Aggregate counts and amounts from the receipts in one pass.
		monthly := make([]float64, 12)
		counts := make([]int, 12)
		currency := ""
		perProvider := map[string]float64{}
		perProviderMonthly := map[string]*[12]float64{}
		for _, it := range items {
			m, y := parsePeriod(it.Period)
			if m < 1 || m > 12 || y != year {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[strings.ToLower(it.Provider)]; !ok {
					continue
				}
			}
			idx := m - 1
			name := strings.ToLower(it.Provider)

			counts[idx]++
			monthly[idx] += it.Amount
			perProvider[name] += it.Amount

			months, ok := perProviderMonthly[name]
			if !ok {
				months = &[12]float64{}
				perProviderMonthly[name] = months
			}
			months[idx] += it.Amount

			if currency == "" && it.Currency != "" {
				currency = it.Currency
			}
		}

		total := 0.0
		for _, v := range monthly {
			total += v
		}

		// Largest spender first, so the UI can render the breakdown as-is.
		byProvider := make([]ProviderTotal, 0, len(perProvider))
		for name, amount := range perProvider {
			byProvider = append(byProvider, ProviderTotal{Provider: name, Amount: amount})
		}
		slices.SortFunc(byProvider, func(a, b ProviderTotal) int {
			if a.Amount != b.Amount {
				return cmp.Compare(b.Amount, a.Amount)
			}

			return strings.Compare(a.Provider, b.Provider)
		})

		// Same order as the totals, so chart lines and table columns line up
		// with the breakdown list.
		monthlyByProvider := make([]ProviderMonthly, 0, len(byProvider))
		for _, pt := range byProvider {
			monthlyByProvider = append(monthlyByProvider, ProviderMonthly{
				Provider: pt.Provider,
				Monthly:  perProviderMonthly[pt.Provider][:],
			})
		}

		// Model
		resp := StatsResponse{
			Year:              year,
			Monthly:           monthly,
			MonthlyCounts:     counts,
			ByProvider:        byProvider,
			MonthlyByProvider: monthlyByProvider,
			Total:             total,
			Average:           total / 12.0,
			Currency:          currency,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Err(err).Msg("Error encoding stats")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// expensesMonthlyAPIHandler returns monthly expenses aggregated from the
// receipts database (price), joined onto the on-disk receipts.
func expensesMonthlyAPIHandler(cfg *config.Config, receiptsStore func() *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := collectReceipts(cfg, nil, receiptsStore())
		if err != nil {
			log.Err(err).Msg("Error scanning receipts for expenses")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Provider filter
		allowed := map[string]struct{}{}
		if providersParam := strings.TrimSpace(r.URL.Query().Get("provider")); providersParam != "" {
			for _, p := range strings.Split(providersParam, ",") {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" {
					allowed[p] = struct{}{}
				}
			}
		}

		// Year param (default to current)
		year := 0
		if yStr := strings.TrimSpace(r.URL.Query().Get("year")); yStr != "" {
			if y, err := strconv.Atoi(yStr); err == nil {
				year = y
			}
		}
		if year == 0 {
			year = time.Now().Year()
		}

		monthly := make([]float64, 12)
		currency := ""

		for _, it := range items {
			mm, yy := parsePeriod(it.Period)
			if mm < 1 || mm > 12 || yy != year {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[strings.ToLower(it.Provider)]; !ok {
					continue
				}
			}
			monthly[mm-1] += it.Amount
			if currency == "" && it.Currency != "" {
				currency = it.Currency
			}
		}

		names := []string{"Jan", "Feb", "Mar", "Apr", "Maj", "Jun", "Jul", "Avg", "Sep", "Okt", "Nov", "Dec"}
		months := make([]map[string]any, 12)
		total := 0.0
		for i := 0; i < 12; i++ {
			amt := monthly[i]
			total += amt
			months[i] = map[string]any{
				"index":  i + 1,
				"name":   names[i],
				"amount": amt,
			}
		}
		avg := total / 12.0

		resp := map[string]any{
			"year":     year,
			"currency": currency,
			"months":   months,
			"total":    total,
			"average":  avg,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Err(err).Msg("Error encoding monthly expenses")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
