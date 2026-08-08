package eventadmin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

const maxMediaBytes = 50 << 20 // 50 MB per file

// allowedExt maps a media kind to the file extensions it accepts.
var allowedExt = map[string]map[string]bool{
	"photo": {".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true},
	"video": {".mp4": true, ".webm": true, ".mov": true},
}

// Media is a photo/video the Event Admin uploaded for an event.
type Media struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// mediaService stores files under <root>/<eventID>/ and records their URLs. The
// local folder is a placeholder for a future cloud object store.
type mediaService struct {
	pool       *pgxpool.Pool
	root       string
	publicBase string
}

func (s mediaService) list(ctx context.Context, adminID, eventID string) ([]Media, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, kind, filename, url FROM admin_event_media WHERE event_id=$1 ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Media{}
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ID, &m.Kind, &m.Filename, &m.URL); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// upload validates the file, writes it into the event's folder and records it.
func (s mediaService) upload(ctx context.Context, adminID, eventID, kind, origName string, data io.Reader) (*Media, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	if allowedExt[kind] == nil {
		return nil, httpx.ErrBadRequest("kind must be photo or video")
	}
	ext := strings.ToLower(filepath.Ext(origName))
	if !allowedExt[kind][ext] {
		return nil, httpx.ErrBadRequest("unsupported file type for " + kind)
	}

	dir := filepath.Join(s.root, eventID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, httpx.ErrInternal("could not create media folder")
	}
	stored := randHex(8) + ext
	dst, err := os.Create(filepath.Join(dir, stored))
	if err != nil {
		return nil, httpx.ErrInternal("could not save file")
	}
	if _, err := io.Copy(dst, data); err != nil {
		dst.Close()
		return nil, httpx.ErrInternal("could not write file")
	}
	dst.Close()

	url := fmt.Sprintf("%s/media/%s/%s", strings.TrimRight(s.publicBase, "/"), eventID, stored)
	m := Media{Kind: kind, Filename: stored, URL: url}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_event_media (event_id, kind, filename, url)
		VALUES ($1,$2,$3,$4) RETURNING id`, eventID, kind, stored, url).Scan(&m.ID); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s mediaService) remove(ctx context.Context, adminID, mediaID string) error {
	// Verify ownership via the event join, and capture the file to delete.
	var eventID, filename string
	err := s.pool.QueryRow(ctx, `
		SELECT m.event_id, m.filename FROM admin_event_media m
		JOIN events e ON e.id = m.event_id
		WHERE m.id=$1 AND e.created_by=$2`, mediaID, adminID).Scan(&eventID, &filename)
	if err != nil {
		return httpx.ErrNotFound("media not found or not yours")
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM admin_event_media WHERE id=$1`, mediaID); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.root, eventID, filename)) // best-effort file cleanup
	return nil
}

func registerMedia(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler, root, publicBase string) {
	svc := mediaService{pool: pool, root: root, publicBase: publicBase}

	mux.Handle("GET /admin/event/events/{id}/media", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events/{id}/media", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		r.Body = http.MaxBytesReader(w, r.Body, maxMediaBytes)
		if err := r.ParseMultipartForm(maxMediaBytes); err != nil {
			httpx.Error(w, httpx.ErrBadRequest("file too large or invalid upload"))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.Error(w, httpx.ErrBadRequest("missing file"))
			return
		}
		defer file.Close()
		out, err := svc.upload(r.Context(), id.AdminID, r.PathValue("id"), r.FormValue("kind"), header.Filename, file)
		respondCreated(w, out, err)
	})))
	mux.Handle("DELETE /admin/event/media/{mediaId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"deleted": true}, svc.remove(r.Context(), id.AdminID, r.PathValue("mediaId")))
	})))
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
