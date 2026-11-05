//go:build notificationflow

// Package notificationflow exercises the running notification worker against a
// disposable local Docker Compose stack. It publishes contract-valid and
// contract-invalid messages to the real exchange, then verifies the real
// worker's effect through Mailpit, the notification schema, and the dead-letter
// queue. It deliberately does not implement a producer, outbox, relay, or any
// other member's service.
package notificationflow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	eventType    = "notification.requested.v1"
	eventVersion = 1
	consumerName = "notification-worker"
)

// harness owns the test configuration and the rows/messages this run created.
type harness struct {
	repoRoot       string
	composeProject string
	rabbitURL      string
	managementURL  string
	managementUser string
	managementPass string
	mailpitURL     string
	exchange       string
	queue          string
	routingKey     string
	retryWait      time.Duration
	httpClient     *http.Client
	eventIDs       []string
	recipients     []string
}

var (
	stackOnce sync.Once
	stackErr  error
)

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		repoRoot:       findRepoRoot(t),
		composeProject: dedicatedComposeProject(t),
		rabbitURL:      envOr("NOTIFICATIONFLOW_RABBITMQ_URL", "amqp://swiftdrop:swiftdrop@127.0.0.1:5672/"),
		managementURL:  strings.TrimRight(envOr("NOTIFICATIONFLOW_RABBITMQ_MANAGEMENT_URL", "http://127.0.0.1:15672"), "/"),
		managementUser: envOr("NOTIFICATIONFLOW_RABBITMQ_USER", "swiftdrop"),
		managementPass: envOr("NOTIFICATIONFLOW_RABBITMQ_PASSWORD", "swiftdrop"),
		mailpitURL:     strings.TrimRight(envOr("NOTIFICATIONFLOW_MAILPIT_URL", "http://127.0.0.1:8025"), "/"),
		exchange:       envOr("NOTIFICATIONFLOW_EXCHANGE", "swiftdrop.events"),
		queue:          envOr("NOTIFICATIONFLOW_QUEUE", "notification.requested.v1"),
		routingKey:     strings.TrimSpace(os.Getenv("NOTIFICATIONFLOW_ROUTING_KEY")),
		retryWait:      envDuration(t, "NOTIFICATIONFLOW_RETRY_WAIT", 60*time.Second),
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
	if h.routingKey == "" {
		h.routingKey = h.queue
	}
	t.Cleanup(h.cleanup)
	return h
}

var composeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// dedicatedComposeProject requires an explicit, non-default Compose project.
// The SMTP outage test stops Mailpit, so the suite must never target the
// developer's normal swiftdrop stack.
func dedicatedComposeProject(t *testing.T) string {
	t.Helper()

	project := strings.TrimSpace(os.Getenv("COMPOSE_PROJECT_NAME"))
	if project == "" {
		t.Fatalf("notificationflow E2E requires an explicit dedicated Compose project. " +
			"Export COMPOSE_PROJECT_NAME=swiftdrop-notificationflow (or another unique name) and use the same value for " +
			"docker compose, make migrate, worker startup, and the tests. The default swiftdrop project is refused " +
			"because the SMTP outage test stops Mailpit.")
	}
	if project == "swiftdrop" {
		t.Fatalf("notificationflow E2E refuses the default Compose project %q because the SMTP outage test stops Mailpit; "+
			"use a dedicated project such as swiftdrop-notificationflow.", project)
	}
	if !composeProjectPattern.MatchString(project) {
		t.Fatalf("COMPOSE_PROJECT_NAME %q is not a valid Compose project name; "+
			"use lowercase letters, digits, hyphens, and underscores, starting with a letter or digit.", project)
	}
	return project
}

// requireStack waits once per test process for the worker, broker, database,
// Mailpit, and notification tables. It fails rather than skips so CI cannot
// report a false pass when a dependency is missing.
func requireStack(t *testing.T, h *harness) {
	t.Helper()
	stackOnce.Do(func() { stackErr = h.waitReady() })
	if stackErr != nil {
		t.Fatalf("notification worker E2E prerequisites are not ready: %v", stackErr)
	}
}

func (h *harness) waitReady() error {
	steps := []struct {
		name    string
		timeout time.Duration
		check   func() error
	}{
		{"RabbitMQ management API", 60 * time.Second, h.pingManagement},
		{"RabbitMQ AMQP", 60 * time.Second, h.pingAMQP},
		{"notification worker queue consumer", 60 * time.Second, h.pingConsumer},
		{"Mailpit", 30 * time.Second, h.pingMailpit},
		{"PostgreSQL", 30 * time.Second, h.pingDatabase},
		{"notification tables", 30 * time.Second, h.pingTables},
	}
	for _, step := range steps {
		if err := waitFor(step.timeout, step.name, step.check); err != nil {
			return err
		}
	}
	return nil
}

func (h *harness) pingManagement() error {
	return h.managementRequest(http.MethodGet, "/api/overview", nil, nil)
}

func (h *harness) pingAMQP() error {
	connection, err := dialAMQP(h.rabbitURL, 5*time.Second)
	if err != nil {
		return err
	}
	return connection.Close()
}

// dialAMQP bounds the TCP dial so a stopped broker cannot hang a readiness
// check past its timeout.
func dialAMQP(rawURL string, timeout time.Duration) (*amqp.Connection, error) {
	dialer := &net.Dialer{Timeout: timeout}
	return amqp.DialConfig(rawURL, amqp.Config{
		Dial: func(network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	})
}

func (h *harness) pingConsumer() error {
	info, err := h.queueInfo(h.queue)
	if err != nil {
		return err
	}
	if info.Consumers < 1 {
		return fmt.Errorf("queue %s has %d consumers; start the notification-worker service", h.queue, info.Consumers)
	}
	return nil
}

func (h *harness) pingMailpit() error {
	response, err := h.httpClient.Get(h.mailpitURL + "/readyz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Mailpit /readyz returned %d", response.StatusCode)
	}
	return nil
}

func (h *harness) pingDatabase() error {
	value, err := h.query("SELECT 1")
	if err != nil {
		return err
	}
	if value != "1" {
		return fmt.Errorf("SELECT 1 returned %q", value)
	}
	return nil
}

func (h *harness) pingTables() error {
	value, err := h.query("SELECT (to_regclass('notification.notification_logs') IS NOT NULL AND to_regclass('notification.processed_events') IS NOT NULL)")
	if err != nil {
		return err
	}
	if value != "t" {
		return fmt.Errorf("notification tables are missing; run `make migrate`")
	}
	return nil
}

// publish sends one message and fails the test unless the broker confirms it
// and routes it. It dials a fresh connection so a broker restart cannot leave
// a stale channel behind.
func (h *harness) publish(t *testing.T, eventID, body string) {
	t.Helper()
	if err := h.publishTo(eventID, body, h.routingKey); err != nil {
		t.Fatalf("%v", err)
	}
}

// publishTo publishes with publisher confirms and mandatory routing. A returned
// message (no matching binding) or a missing confirm is an error, so callers
// cannot mistake a dropped message for a successful publish.
func (h *harness) publishTo(eventID, body, routingKey string) error {
	connection, err := dialAMQP(h.rabbitURL, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	defer connection.Close()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	defer channel.Close()

	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(ctx, h.exchange, routingKey, true, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     eventID,
		CorrelationId: "notificationflow-" + eventID,
		Body:          []byte(body),
	})
	if err != nil {
		return fmt.Errorf("publish event %s to %s/%s: %w", eventID, h.exchange, routingKey, err)
	}

	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for publisher confirm for event %s to %s/%s: %w", eventID, h.exchange, routingKey, err)
	}
	if !confirmed {
		return fmt.Errorf("broker did not confirm event %s to %s/%s", eventID, h.exchange, routingKey)
	}

	select {
	case returned, ok := <-returns:
		if ok {
			return fmt.Errorf("message for event %s was returned from %s/%s: %d %s",
				eventID, h.exchange, routingKey, returned.ReplyCode, returned.ReplyText)
		}
	default:
	}

	return nil
}

// queueInfo reads queue depth and consumer count through the RabbitMQ
// management API. Using the management API avoids closing an AMQP channel when
// the queue does not exist yet during readiness checks.
type queueInfo struct {
	Consumers int `json:"consumers"`
	Messages  int `json:"messages"`
}

func (h *harness) queueInfo(queue string) (queueInfo, error) {
	var info queueInfo
	path := "/api/queues/%2F/" + url.PathEscape(queue)
	if err := h.managementRequest(http.MethodGet, path, nil, &info); err != nil {
		return queueInfo{}, err
	}
	return info, nil
}

type dlqMessage struct {
	Payload    string `json:"payload"`
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routing_key"`
	Properties struct {
		Headers map[string]any `json:"headers"`
	} `json:"properties"`
}

// dlqPeek reads dead-letter messages without consuming them. ack_requeue_true
// keeps the queue intact, so repeated runs can still inspect their own marker.
func (h *harness) dlqPeek(t *testing.T) []dlqMessage {
	t.Helper()

	var messages []dlqMessage
	path := "/api/queues/%2F/" + url.PathEscape(h.queue+".dlq") + "/get"
	body := map[string]any{
		"count":    1000,
		"ackmode":  "ack_requeue_true",
		"encoding": "auto",
	}
	if err := h.managementRequest(http.MethodPost, path, body, &messages); err != nil {
		t.Fatalf("peek dead-letter queue: %v", err)
	}
	return messages
}

func (h *harness) managementRequest(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode management request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, h.managementURL+path, reader)
	if err != nil {
		return err
	}
	request.SetBasicAuth(h.managementUser, h.managementPass)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := h.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("%s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(message)))
	}
	if out != nil {
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			return fmt.Errorf("decode management response: %w", err)
		}
	}
	return nil
}

type mailpitSummary struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
	To      []struct {
		Address string `json:"Address"`
	} `json:"To"`
}

type mailpitMessage struct {
	Subject string `json:"Subject"`
	Text    string `json:"Text"`
	HTML    string `json:"HTML"`
	To      []struct {
		Address string `json:"Address"`
	} `json:"To"`
}

func (h *harness) mailpitSearch(recipient string) ([]mailpitSummary, error) {
	var list struct {
		Messages []mailpitSummary `json:"messages"`
	}
	path := h.mailpitURL + "/api/v1/search?query=" + url.QueryEscape("to:"+recipient)
	response, err := h.httpClient.Get(path)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("Mailpit search returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode Mailpit search: %w", err)
	}
	return list.Messages, nil
}

func (h *harness) mailpitMessage(id string) (mailpitMessage, error) {
	var message mailpitMessage
	response, err := h.httpClient.Get(h.mailpitURL + "/api/v1/message/" + url.PathEscape(id))
	if err != nil {
		return mailpitMessage{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return mailpitMessage{}, fmt.Errorf("Mailpit message returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		return mailpitMessage{}, fmt.Errorf("decode Mailpit message: %w", err)
	}
	return message, nil
}

func (h *harness) mailpitDelete(recipient string) error {
	path := h.mailpitURL + "/api/v1/search?query=" + url.QueryEscape("to:"+recipient)
	request, err := http.NewRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	response, err := h.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("Mailpit delete returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (h *harness) mailpitCount(t *testing.T, recipient string) int {
	t.Helper()
	messages, err := h.mailpitSearch(recipient)
	if err != nil {
		t.Fatalf("search Mailpit for %s: %v", recipient, err)
	}
	return len(messages)
}

func (h *harness) waitForMailpit(t *testing.T, recipient string, want int, timeout time.Duration) []mailpitSummary {
	t.Helper()

	err := waitFor(timeout, fmt.Sprintf("Mailpit messages for %s", recipient), func() error {
		messages, err := h.mailpitSearch(recipient)
		if err != nil {
			return err
		}
		if len(messages) < want {
			return fmt.Errorf("got %d messages, want at least %d", len(messages), want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	messages, err := h.mailpitSearch(recipient)
	if err != nil {
		t.Fatalf("search Mailpit for %s: %v", recipient, err)
	}
	return messages
}

// query runs one statement through the Compose PostgreSQL service. It never
// publishes a database port, so the test works with the repository's default
// network topology.
func (h *harness) query(statement string) (string, error) {
	args := append(h.composeArgs(), "exec", "-T", "postgres", "sh", "-c",
		`exec psql -v ON_ERROR_STOP=1 -At -q -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)

	command := exec.Command("docker", args...)
	command.Dir = h.repoRoot
	command.Stdin = strings.NewReader(statement)

	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("psql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(output)), nil
}

func (h *harness) scalar(t *testing.T, statement string) string {
	t.Helper()
	value, err := h.query(statement)
	if err != nil {
		t.Fatalf("query %q: %v", statement, err)
	}
	return value
}

func (h *harness) count(t *testing.T, statement string) int {
	t.Helper()
	value := h.scalar(t, statement)
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse count %q from %q: %v", value, statement, err)
	}
	return parsed
}

func (h *harness) waitForStatus(t *testing.T, eventID, recipient, want string, timeout time.Duration) {
	t.Helper()

	statement := "SELECT status FROM notification.notification_logs WHERE event_id = " +
		sqlLiteral(eventID) + " AND recipient = " + sqlLiteral(recipient)
	err := waitFor(timeout, "notification log status "+want, func() error {
		got, err := h.query(statement)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("status %q, want %q", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *harness) waitForProcessed(t *testing.T, eventID string, want int, timeout time.Duration) {
	t.Helper()

	statement := "SELECT count(*) FROM notification.processed_events WHERE consumer = " +
		sqlLiteral(consumerName) + " AND event_id = " + sqlLiteral(eventID)
	err := waitFor(timeout, "processed event count", func() error {
		got, err := h.query(statement)
		if err != nil {
			return err
		}
		if got != strconv.Itoa(want) {
			return fmt.Errorf("processed count %q, want %d", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *harness) stopMailpit(t *testing.T) {
	t.Helper()
	if err := h.composeErr("stop", "mailpit"); err != nil {
		t.Fatalf("stop Mailpit: %v", err)
	}
}

func (h *harness) startMailpit(t *testing.T) {
	t.Helper()
	if err := h.composeErr("start", "mailpit"); err != nil {
		t.Fatalf("start Mailpit: %v", err)
	}
	if err := waitFor(30*time.Second, "Mailpit", h.pingMailpit); err != nil {
		t.Fatalf("start Mailpit: %v", err)
	}
}

// startMailpitBestEffort is used from t.Cleanup, where t.Fatalf would abort the
// remaining cleanup work.
func (h *harness) startMailpitBestEffort() {
	if err := h.composeErr("start", "mailpit"); err != nil {
		fmt.Fprintf(os.Stderr, "notificationflow: restart Mailpit: %v\n", err)
		return
	}
	if err := waitFor(30*time.Second, "Mailpit", h.pingMailpit); err != nil {
		fmt.Fprintf(os.Stderr, "notificationflow: wait for Mailpit: %v\n", err)
	}
}

func (h *harness) composeErr(args ...string) error {
	command := exec.Command("docker", append(h.composeArgs(), args...)...)
	command.Dir = h.repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (h *harness) composeLogs(service string) (string, error) {
	command := exec.Command("docker", append(h.composeArgs(), "logs", "--no-color", "--tail=200", service)...)
	command.Dir = h.repoRoot

	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("docker compose logs %s: %w: %s", service, err, strings.TrimSpace(stderr.String()))
	}
	return string(output), nil
}

// waitForAlreadyProcessedLog proves the worker consumed the duplicate and hit
// the existing processed-event path. The worker logs the event ID and
// "notification already processed" on the same line.
func (h *harness) waitForAlreadyProcessedLog(t *testing.T, eventID string, timeout time.Duration) {
	t.Helper()

	err := waitFor(timeout, "worker to log notification already processed for "+eventID, func() error {
		logs, err := h.composeLogs("notification-worker")
		if err != nil {
			return err
		}
		for _, line := range strings.Split(logs, "\n") {
			if strings.Contains(line, eventID) && strings.Contains(line, "notification already processed") {
				return nil
			}
		}
		return fmt.Errorf("worker has not logged notification already processed for event %s yet", eventID)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *harness) composeArgs() []string {
	return []string{"compose", "-p", h.composeProject}
}

func (h *harness) trackEvent(eventID string) {
	h.eventIDs = append(h.eventIDs, eventID)
}

func (h *harness) trackRecipient(recipient string) {
	h.recipients = append(h.recipients, recipient)
}

// cleanup removes only the rows and Mailpit messages this run created. It
// never drops a database, deletes a volume, purges a shared queue, or removes
// the worker's topology.
func (h *harness) cleanup() {
	for _, recipient := range h.recipients {
		if err := h.mailpitDelete(recipient); err != nil {
			fmt.Fprintf(os.Stderr, "notificationflow: delete Mailpit messages for %s: %v\n", recipient, err)
		}
	}

	if len(h.eventIDs) == 0 {
		return
	}
	ids := make([]string, 0, len(h.eventIDs))
	for _, eventID := range h.eventIDs {
		ids = append(ids, sqlLiteral(eventID))
	}
	list := strings.Join(ids, ",")
	statement := fmt.Sprintf(
		`DELETE FROM notification.notification_logs WHERE event_id IN (%[1]s);
		 DELETE FROM notification.processed_events WHERE consumer = %[2]s AND event_id IN (%[1]s);`,
		list, sqlLiteral(consumerName),
	)
	if _, err := h.query(statement); err != nil {
		fmt.Fprintf(os.Stderr, "notificationflow: delete test rows: %v\n", err)
	}
}

func waitFor(timeout time.Duration, description string, check func() error) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if err := check(); err == nil {
			return nil
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("%s not ready within %s: %v", description, timeout, last)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find the repository root (go.mod)")
		}
		directory = parent
	}
}

func (h *harness) newUUID(t *testing.T) string {
	t.Helper()

	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func (h *harness) runID(t *testing.T) string {
	t.Helper()

	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate run ID: %v", err)
	}
	return hex.EncodeToString(value[:])
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		t.Fatalf("%s must be a duration: %v", key, err)
	}
	return parsed
}
