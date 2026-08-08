package operationaladmin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

type opsPart struct {
	Name, Contact, Aadhaar, Category, Entry, Payment string
}
type opsCrewRow struct {
	Name, Role, Contact string
	Cost                float64
}
type opsExpense struct {
	Amount        float64
	Comment, Date string
}
type opsVendorRow struct {
	Name        string  `json:"name"`
	ServiceType string  `json:"serviceType"`
	Cost        float64 `json:"cost"`
}
type opsSponsorRow struct {
	Organisation string  `json:"organisation"`
	Tier         string  `json:"tier"`
	Cost         float64 `json:"cost"`
}

// opsReportData is everything a full Ops event report / archive contains.
type opsReportData struct {
	EventName    string          `json:"eventName"`
	City         string          `json:"city"`
	Participants []opsPart       `json:"participants"`
	Crew         []opsCrewRow    `json:"crew"`
	Expenses     []opsExpense    `json:"expenses"`
	Vendors      []opsVendorRow  `json:"vendors"`
	Sponsors     []opsSponsorRow `json:"sponsors"`
	PnL          PnL             `json:"pnl"`
}

type opsReportService struct{ pool *pgxpool.Pool }

func (s opsReportService) gather(ctx context.Context, eventID string) (*opsReportData, error) {
	d := &opsReportData{}
	err := s.pool.QueryRow(ctx, `SELECT name, city FROM events WHERE id=$1`, eventID).Scan(&d.EventName, &d.City)
	if err == pgx.ErrNoRows {
		return nil, httpx.ErrNotFound("event not found")
	}
	if err != nil {
		return nil, err
	}

	prows, err := s.pool.Query(ctx, `
		SELECT p.name, f.phone, COALESCE(p.aadhaar_masked,''), c.name, en.entry_code, r.status
		FROM entries en
		JOIN registrations r ON r.id = en.registration_id
		JOIN people p ON p.id = en.person_id
		JOIN families f ON f.id = p.family_id
		JOIN event_categories ec ON ec.id = en.event_category_id
		JOIN admin_categories c ON c.id = ec.category_id
		WHERE r.event_id=$1 ORDER BY p.name`, eventID)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var p opsPart
		if err := prows.Scan(&p.Name, &p.Contact, &p.Aadhaar, &p.Category, &p.Entry, &p.Payment); err != nil {
			prows.Close()
			return nil, err
		}
		d.Participants = append(d.Participants, p)
	}
	prows.Close()

	crows, err := s.pool.Query(ctx,
		`SELECT name, role, COALESCE(contact,''), cost FROM admin_event_crew WHERE event_id=$1 ORDER BY role, name`, eventID)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var c opsCrewRow
		if err := crows.Scan(&c.Name, &c.Role, &c.Contact, &c.Cost); err != nil {
			crows.Close()
			return nil, err
		}
		d.Crew = append(d.Crew, c)
	}
	crows.Close()

	erows, err := s.pool.Query(ctx,
		`SELECT amount, comment, to_char(created_at,'YYYY-MM-DD') FROM admin_event_expenses WHERE event_id=$1 ORDER BY created_at`, eventID)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var e opsExpense
		if err := erows.Scan(&e.Amount, &e.Comment, &e.Date); err != nil {
			erows.Close()
			return nil, err
		}
		d.Expenses = append(d.Expenses, e)
	}
	erows.Close()

	vrows, err := s.pool.Query(ctx,
		`SELECT name, service_type, cost FROM admin_event_vendors WHERE event_id=$1 ORDER BY name`, eventID)
	if err != nil {
		return nil, err
	}
	for vrows.Next() {
		var v opsVendorRow
		if err := vrows.Scan(&v.Name, &v.ServiceType, &v.Cost); err != nil {
			vrows.Close()
			return nil, err
		}
		d.Vendors = append(d.Vendors, v)
	}
	vrows.Close()

	srows, err := s.pool.Query(ctx,
		`SELECT organisation, tier, cost FROM admin_event_sponsors WHERE event_id=$1 ORDER BY organisation`, eventID)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var sp opsSponsorRow
		if err := srows.Scan(&sp.Organisation, &sp.Tier, &sp.Cost); err != nil {
			srows.Close()
			return nil, err
		}
		d.Sponsors = append(d.Sponsors, sp)
	}
	srows.Close()

	pnl, err := (eventOpsService{s.pool}).pnl(ctx, eventID)
	if err != nil {
		return nil, err
	}
	d.PnL = *pnl
	return d, nil
}

// purge deletes every trace of the event across both domains, in one transaction.
func (s opsReportService) purge(ctx context.Context, eventID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, q := range []string{
		`DELETE FROM entries WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM payments WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM registrations WHERE event_id=$1`,
		`DELETE FROM certificates WHERE event_id=$1`,
		`DELETE FROM feedback WHERE event_id=$1`,
		`DELETE FROM event_categories WHERE event_id=$1`,
		`DELETE FROM admin_event_crew WHERE event_id=$1`,
		`DELETE FROM admin_event_expenses WHERE event_id=$1`,
		`DELETE FROM admin_event_vendors WHERE event_id=$1`,
		`DELETE FROM admin_event_sponsors WHERE event_id=$1`,
		`DELETE FROM admin_notifications WHERE event_id=$1`,
		`DELETE FROM admin_notification_config WHERE event_id=$1`,
		`DELETE FROM events WHERE id=$1`,
	} {
		if _, err := tx.Exec(ctx, q, eventID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func registerOpsReport(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := opsReportService{pool}

	mux.Handle("GET /admin/ops/events/{id}/report", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := svc.gather(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.Error(w, err)
			return
		}
		if r.URL.Query().Get("format") == "pdf" {
			opsWritePDF(w, d)
			return
		}
		opsWriteCSV(w, d)
	})))

	// Archive builds the download first, then purges — so the data is captured
	// before it is deleted.
	mux.Handle("POST /admin/ops/events/{id}/archive", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventID := r.PathValue("id")
		d, err := svc.gather(r.Context(), eventID)
		if err != nil {
			httpx.Error(w, err)
			return
		}
		archive, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			httpx.Error(w, httpx.ErrInternal("could not build archive"))
			return
		}
		if err := svc.purge(r.Context(), eventID); err != nil {
			httpx.Error(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="stagex-archive.json"`)
		_, _ = w.Write(archive)
	})))
}

func opsWriteCSV(w http.ResponseWriter, d *opsReportData) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stagex-ops-report.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"StageX Event Report (Ops)"})
	_ = cw.Write([]string{"Event", d.EventName})
	_ = cw.Write([]string{"City", d.City})
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"P&L"})
	_ = cw.Write([]string{"Revenue (registrations)", money(d.PnL.Revenue)})
	_ = cw.Write([]string{"Sponsor income", money(d.PnL.SponsorIncome)})
	_ = cw.Write([]string{"Vendor income", money(d.PnL.VendorIncome)})
	_ = cw.Write([]string{"Total income", money(d.PnL.TotalIncome)})
	_ = cw.Write([]string{"Crew cost", money(d.PnL.CrewCost)})
	_ = cw.Write([]string{"Other expenses", money(d.PnL.Expenses)})
	_ = cw.Write([]string{"Hall cost", money(d.PnL.HallCost)})
	_ = cw.Write([]string{"Total expenses", money(d.PnL.TotalExpenses)})
	_ = cw.Write([]string{"Net P&L", money(d.PnL.NetPL)})
	_ = cw.Write([]string{"Margin %", fmt.Sprintf("%.1f", d.PnL.Margin)})
	_ = cw.Write([]string{"Participants", fmt.Sprintf("%d", d.PnL.Participants)})
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Participants"})
	_ = cw.Write([]string{"Name", "Contact", "Aadhaar", "Category", "Entry ID", "Payment"})
	for _, p := range d.Participants {
		_ = cw.Write([]string{p.Name, p.Contact, p.Aadhaar, p.Category, p.Entry, p.Payment})
	}
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Crew"})
	_ = cw.Write([]string{"Name", "Role", "Contact", "Cost"})
	for _, c := range d.Crew {
		_ = cw.Write([]string{c.Name, c.Role, c.Contact, money(c.Cost)})
	}
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Additional expenses"})
	_ = cw.Write([]string{"Amount", "Comment", "Date"})
	for _, e := range d.Expenses {
		_ = cw.Write([]string{money(e.Amount), e.Comment, e.Date})
	}
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Vendors (income)"})
	_ = cw.Write([]string{"Name", "Service", "Cost"})
	for _, v := range d.Vendors {
		_ = cw.Write([]string{v.Name, v.ServiceType, money(v.Cost)})
	}
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Sponsors (income)"})
	_ = cw.Write([]string{"Organisation", "Tier", "Cost"})
	for _, sp := range d.Sponsors {
		_ = cw.Write([]string{sp.Organisation, sp.Tier, money(sp.Cost)})
	}
}

func opsWritePDF(w http.ResponseWriter, d *opsReportData) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("StageX Ops Event Report", false)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "StageX — Event Report (Ops)", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("Event: %s   City: %s", d.EventName, d.City), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// P&L block.
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, "Profit & Loss", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	for _, kv := range [][2]string{
		{"Revenue (registrations)", money(d.PnL.Revenue)},
		{"Sponsor income", money(d.PnL.SponsorIncome)},
		{"Vendor income", money(d.PnL.VendorIncome)},
		{"Total income", money(d.PnL.TotalIncome)},
		{"Crew cost", money(d.PnL.CrewCost)},
		{"Other expenses", money(d.PnL.Expenses)},
		{"Hall cost", money(d.PnL.HallCost)},
		{"Total expenses", money(d.PnL.TotalExpenses)},
		{"Net P&L", money(d.PnL.NetPL)},
		{"Margin", fmt.Sprintf("%.1f%%", d.PnL.Margin)},
		{"Participants", fmt.Sprintf("%d", d.PnL.Participants)},
	} {
		pdf.CellFormat(60, 6, kv[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, kv[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(3)

	section := func(title string, header []string, widths []float64, rows [][]string) {
		pdf.SetFont("Arial", "B", 13)
		pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "B", 9)
		pdf.SetFillColor(242, 233, 251)
		for i, h := range header {
			pdf.CellFormat(widths[i], 7, h, "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Arial", "", 9)
		if len(rows) == 0 {
			pdf.CellFormat(0, 7, "— none —", "1", 1, "L", false, 0, "")
		}
		for _, row := range rows {
			for i, cell := range row {
				pdf.CellFormat(widths[i], 7, opsTrunc(cell, widths[i]), "1", 0, "L", false, 0, "")
			}
			pdf.Ln(-1)
		}
		pdf.Ln(4)
	}

	prows := make([][]string, 0, len(d.Participants))
	for _, p := range d.Participants {
		prows = append(prows, []string{p.Name, p.Contact, p.Aadhaar, p.Category, p.Entry, p.Payment})
	}
	section("Participants", []string{"Name", "Contact", "Aadhaar", "Category", "Entry", "Payment"},
		[]float64{40, 28, 30, 35, 25, 22}, prows)

	crows := make([][]string, 0, len(d.Crew))
	for _, c := range d.Crew {
		crows = append(crows, []string{c.Name, c.Role, c.Contact, money(c.Cost)})
	}
	section("Crew", []string{"Name", "Role", "Contact", "Cost"}, []float64{50, 45, 50, 35}, crows)

	erows := make([][]string, 0, len(d.Expenses))
	for _, e := range d.Expenses {
		erows = append(erows, []string{money(e.Amount), e.Comment, e.Date})
	}
	section("Additional expenses", []string{"Amount", "Comment", "Date"}, []float64{35, 110, 35}, erows)

	vrows := make([][]string, 0, len(d.Vendors))
	for _, v := range d.Vendors {
		vrows = append(vrows, []string{v.Name, v.ServiceType, money(v.Cost)})
	}
	section("Vendors (income)", []string{"Name", "Service", "Cost"}, []float64{70, 70, 40}, vrows)

	srows := make([][]string, 0, len(d.Sponsors))
	for _, sp := range d.Sponsors {
		srows = append(srows, []string{sp.Organisation, sp.Tier, money(sp.Cost)})
	}
	section("Sponsors (income)", []string{"Organisation", "Tier", "Cost"}, []float64{80, 60, 40}, srows)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="stagex-ops-report.pdf"`)
	_ = pdf.Output(w)
}

func money(v float64) string { return fmt.Sprintf("%.2f", v) }

func opsTrunc(s string, width float64) string {
	max := int(width / 1.8)
	if max > 3 && len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
