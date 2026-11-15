// Command webhook-sink is a deterministic receiver for development-only
// webhook retry tests. It listens on HTTPS :8098 for the worker and HTTP :8099
// for test control. It is started only through the webhook-test Compose profile.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type recordedRequest struct {
	Method     string              `json:"method"`
	URL        string              `json:"url"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	ReceivedAt string              `json:"receivedAt"`
	Status     int                 `json:"status"`
}

type sink struct {
	mu            sync.Mutex
	statuses      []int
	defaultStatus int
	requests      []recordedRequest
}

func main() {
	s := &sink{defaultStatus: http.StatusOK}
	control := http.NewServeMux()
	control.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	control.HandleFunc("/control", s.control)
	control.HandleFunc("/reset", s.reset)
	go func() {
		log.Fatal(http.ListenAndServe(":8099", control))
	}()

	certificate, err := selfSignedCertificate()
	if err != nil {
		log.Fatal(err)
	}
	webhook := http.NewServeMux()
	webhook.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	webhook.HandleFunc("/", s.deliver)
	server := &http.Server{
		Addr:              ":8098",
		Handler:           webhook,
		ReadHeaderTimeout: 5 * time.Second,
	}
	tlsListener, err := tls.Listen("tcp", server.Addr, &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(server.Serve(tlsListener))
}

func (s *sink) control(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"sequence":      s.statuses,
			"defaultStatus": s.defaultStatus,
			"requests":      s.requests,
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Statuses      []int `json:"statuses"`
		DefaultStatus *int  `json:"defaultStatus"`
		Reset         bool  `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if body.Reset {
		s.requests = nil
	}
	if body.Statuses != nil {
		for _, status := range body.Statuses {
			if status < 100 || status > 599 {
				http.Error(w, "statuses must be HTTP status codes", http.StatusBadRequest)
				return
			}
		}
		s.statuses = append([]int(nil), body.Statuses...)
	}
	if body.DefaultStatus != nil {
		if *body.DefaultStatus < 100 || *body.DefaultStatus > 599 {
			http.Error(w, "defaultStatus must be an HTTP status code", http.StatusBadRequest)
			return
		}
		s.defaultStatus = *body.DefaultStatus
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sequence":      s.statuses,
		"defaultStatus": s.defaultStatus,
	})
}

func (s *sink) reset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.statuses = nil
	s.defaultStatus = http.StatusOK
	s.requests = nil
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (s *sink) deliver(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	s.mu.Lock()
	status := s.defaultStatus
	if len(s.statuses) > 0 {
		status = s.statuses[0]
		s.statuses = s.statuses[1:]
	}
	headers := make(map[string][]string, len(r.Header))
	for key, values := range r.Header {
		headers[strings.ToLower(key)] = append([]string(nil), values...)
	}
	s.requests = append(s.requests, recordedRequest{
		Method:     r.Method,
		URL:        r.URL.String(),
		Headers:    headers,
		Body:       string(body),
		ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Status:     status,
	})
	s.mu.Unlock()
	writeJSON(w, status, map[string]bool{"accepted": status >= 200 && status < 300})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func selfSignedCertificate() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "webhook-sink"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"webhook-sink", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return tls.X509KeyPair(certPEM, keyPEM)
}
