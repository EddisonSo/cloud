package cloudflare

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchZone(t *testing.T) {
	zones := []Zone{
		{ID: "z1", Name: "eddisonso2.com"},
		{ID: "z2", Name: "sub.eddisonso2.com"},
		{ID: "z3", Name: "other.io"},
	}
	cases := []struct {
		domain string
		wantID string
		wantOK bool
	}{
		{"resume.eddisonso2.com", "z1", true},
		{"a.sub.eddisonso2.com", "z2", true}, // longest suffix wins
		{"eddisonso2.com", "z1", true},       // zone apex exact match
		{"noteddisonso2.com", "", false},     // suffix must be dot-separated
		{"example.org", "", false},
	}
	for _, c := range cases {
		id, ok := matchZone(c.domain, zones)
		if ok != c.wantOK || id != c.wantID {
			t.Errorf("matchZone(%q) = (%q,%v), want (%q,%v)", c.domain, id, ok, c.wantID, c.wantOK)
		}
	}
}

func stub(t *testing.T, handler http.HandlerFunc) *Client {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{token: "tok", baseURL: srv.URL, http: srv.Client()}
}

func envelope(result any) []byte {
	b, _ := json.Marshal(map[string]any{"success": true, "errors": []any{}, "result": result})
	return b
}

func TestListZonesAndFindZone(t *testing.T) {
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		w.Write(envelope([]Zone{{ID: "z1", Name: "eddisonso2.com"}}))
	})
	zs, err := c.ListZones()
	if err != nil || len(zs) != 1 || zs[0].Name != "eddisonso2.com" {
		t.Fatalf("ListZones: %v %+v", err, zs)
	}
	id, err := c.FindZone("resume.eddisonso2.com")
	if err != nil || id != "z1" {
		t.Fatalf("FindZone: %v %q", err, id)
	}
	if _, err := c.FindZone("nope.example.org"); !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("want ErrZoneNotFound, got %v", err)
	}
}

func TestUpsertCNAMECreates(t *testing.T) {
	var created dnsRecord
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Write(envelope([]dnsRecord{}))
		case "POST":
			json.NewDecoder(r.Body).Decode(&created)
			w.Write(envelope(created))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	if err := c.UpsertCNAME("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.Type != "CNAME" || created.Proxied != false || created.Content != "ingress.cloud.eddisonso.com" {
		t.Errorf("bad create payload: %+v", created)
	}
}

func TestUpsertCNAMEUpdatesExisting(t *testing.T) {
	var updated dnsRecord
	var putPath string
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET": // existing orange-cloud record gets repaired
			w.Write(envelope([]dnsRecord{{ID: "rec9", Type: "CNAME", Name: "resume.eddisonso2.com", Content: "x", Proxied: true}}))
		case "PUT":
			putPath = r.URL.Path
			json.NewDecoder(r.Body).Decode(&updated)
			w.Write(envelope(updated))
		default:
			t.Errorf("unexpected %s", r.Method)
		}
	})
	if err := c.UpsertCNAME("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if updated.Proxied != false || putPath != "/zones/z1/dns_records/rec9" {
		t.Errorf("update wrong: proxied=%v path=%s", updated.Proxied, putPath)
	}
}

func TestDeleteRecordGuard(t *testing.T) {
	deleted := false
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET": // content does NOT match the expected target
			w.Write(envelope([]dnsRecord{{ID: "rec1", Content: "somewhere-else.example"}}))
		case "DELETE":
			deleted = true
			w.Write(envelope(nil))
		}
	})
	if err := c.DeleteRecord("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted {
		t.Error("must NOT delete a record whose content differs from expected")
	}
}

func TestAPIErrorSurfaces(t *testing.T) {
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"auth error"}],"result":null}`))
	})
	if _, err := c.ListZones(); err == nil {
		t.Fatal("expected error from success:false envelope")
	}
}
