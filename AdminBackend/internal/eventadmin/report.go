package eventadmin

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// reportData is the full offline report for an event: participants (with name,
// contact and masked Aadhaar) and the assigned crew (Admin Design §5.3).
type reportData struct {
	EventName string
	City      string
	Parts     [][]string // Name, Contact, Aadhaar, Category, Entry ID, Payment
	Crew      [][]string // Name, Role, Contact
}

type reportService struct{ pool *pgxpool.Pool }

func (s reportService) gather(ctx context.Context, adminID, eventID string) (*reportData, error) {
	d := &reportData{}
	err := s.pool.QueryRow(ctx,
		`SELECT name, city FROM events WHERE id=$1 AND created_by=$2`, eventID, adminID).
		Scan(&d.EventName, &d.City)
	if err == pgx.ErrNoRows {
		return nil, httpx.ErrNotFound("event not found or not yours")
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
	defer prows.Close()
	for prows.Next() {
		r := make([]string, 6)
		if err := prows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4], &r[5]); err != nil {
			return nil, err
		}
		d.Parts = append(d.Parts, r)
	}

	crows, err := s.pool.Query(ctx,
		`SELECT name, role, COALESCE(contact,'') FROM admin_event_crew WHERE event_id=$1 ORDER BY role, name`, eventID)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		r := make([]string, 3)
		if err := crows.Scan(&r[0], &r[1], &r[2]); err != nil {
			return nil, err
		}
		d.Crew = append(d.Crew, r)
	}
	return d, nil
}

func registerReport(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := reportService{pool}
	mux.Handle("GET /admin/event/events/{id}/report", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		eventID := r.PathValue("id")
		data, err := svc.gather(r.Context(), id.AdminID, eventID)
		if err != nil {
			httpx.Error(w, err) // safe: nothing written yet
			return
		}
		if r.URL.Query().Get("format") == "pdf" {
			writePDF(w, data)
			return
		}
		writeCSV(w, data)
	})))
}

func writeCSV(w http.ResponseWriter, d *reportData) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stagex-event-report.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"StageX Event Report"})
	_ = cw.Write([]string{"Event", d.EventName})
	_ = cw.Write([]string{"City", d.City})
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Participants"})
	_ = cw.Write([]string{"Name", "Contact", "Aadhaar", "Category", "Entry ID", "Payment"})
	for _, r := range d.Parts {
		_ = cw.Write(r)
	}
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Crew"})
	_ = cw.Write([]string{"Name", "Role", "Contact"})
	for _, r := range d.Crew {
		_ = cw.Write(r)
	}
}

func writePDF(w http.ResponseWriter, d *reportData) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("StageX Event Report", false)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "StageX — Event Report", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("Event: %s   City: %s", d.EventName, d.City), "", 1, "L", false, 0, "")
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
				pdf.CellFormat(widths[i], 7, truncate(cell, widths[i]), "1", 0, "L", false, 0, "")
			}
			pdf.Ln(-1)
		}
		pdf.Ln(4)
	}

	section("Participants",
		[]string{"Name", "Contact", "Aadhaar", "Category", "Entry ID", "Payment"},
		[]float64{40, 28, 30, 35, 25, 22}, d.Parts)
	section("Crew",
		[]string{"Name", "Role", "Contact"},
		[]float64{60, 60, 60}, d.Crew)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="stagex-event-report.pdf"`)
	_ = pdf.Output(w)
}

// truncate keeps a cell's text within its column width (rough char estimate).
func truncate(s string, width float64) string {
	max := int(width / 1.8)
	if max > 3 && len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
