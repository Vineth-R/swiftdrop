// Command preview renders every SwiftDrop email with sample data, so the
// templates can be reviewed without running a service.
//
//	go run ./packages/go-common/emails/cmd/preview            # writes tmp/email-preview/
//	go run ./packages/go-common/emails/cmd/preview -send      # also sends them to Mailpit
//
// Open tmp/email-preview/index.html to see all of them, or Mailpit
// (http://localhost:8025) after -send to see them the way a mail app does.
//
// To prove real delivery with your own provider, send them to an inbox you own
// using the same SMTP_* settings the services read (the password stays in the
// environment, never on the command line):
//
//	SMTP_HOST=smtp.gmail.com SMTP_PORT=587 SMTP_USER=you@gmail.com \
//	SMTP_PASSWORD=app-password SMTP_FROM=you@gmail.com \
//	  go run ./packages/go-common/emails/cmd/preview -env -send -to you@gmail.com
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

func main() {
	out := flag.String("out", "tmp/email-preview", "directory to write the HTML and text files to")
	baseURL := flag.String("base-url", "http://localhost:8080", "web app address the links point to")
	support := flag.String("support", "support@swiftdrop.example", "support address shown in the footer")
	send := flag.Bool("send", false, "also send every email over SMTP")
	fromEnv := flag.Bool("env", false, "read the SMTP settings from SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASSWORD, SMTP_FROM, SMTP_FROM_NAME, SMTP_REPLY_TO and SMTP_SECURE instead of -smtp-host and -smtp-port")
	to := flag.String("to", "preview@swiftdrop.test", "recipient when sending")
	host := flag.String("smtp-host", "localhost", "SMTP host when sending (Mailpit in Docker Compose)")
	port := flag.Int("smtp-port", 1025, "SMTP port when sending")
	flag.Parse()

	config := mailer.Config{
		Host: *host, Port: *port, From: "no-reply@swiftdrop.local", FromName: "SwiftDrop", ReplyTo: *support,
	}
	if *fromEnv {
		loaded, err := mailer.LoadConfig(sharedconfig.Load())
		if err != nil {
			fmt.Fprintln(os.Stderr, "preview:", err)
			os.Exit(1)
		}
		config = loaded
		if config.ReplyTo == "" {
			config.ReplyTo = *support
		}
	}

	if err := run(*out, emails.Site{BaseURL: *baseURL, SupportEmail: *support}, *send, *to, config); err != nil {
		fmt.Fprintln(os.Stderr, "preview:", err)
		os.Exit(1)
	}
}

func run(out string, site emails.Site, send bool, to string, config mailer.Config) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	var sender *mailer.SMTP
	if send {
		var err error
		sender, err = mailer.NewSMTP(config)
		if err != nil {
			return err
		}
	}

	var index strings.Builder
	index.WriteString(`<!doctype html><meta charset="utf-8"><title>SwiftDrop emails</title>
<style>body{margin:0;font:15px/1.5 system-ui,sans-serif;background:#f6f4ef;color:#14171a}
main{max-width:1500px;margin:0 auto;padding:32px 24px}h1{margin:0 0 4px}p.sub{margin:0 0 24px;color:#5c6068}
h2{margin:32px 0 12px}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(420px,1fr));gap:20px}
figure{margin:0;background:#fff;border:1px solid #dad6cb;border-radius:12px;overflow:hidden}
figcaption{padding:12px 16px;border-bottom:1px solid #dad6cb}figcaption b{display:block}
figcaption span{color:#5c6068;font-size:13px}iframe{width:100%;height:760px;border:0;background:#f6f4ef}
a{color:#0f6b6b}</style><main><h1>SwiftDrop emails</h1>
<p class="sub">Every email the system sends, with sample data. Each is also saved as .html and .txt next to this page.</p>`)

	group := ""
	count := 0
	for _, entry := range emails.Catalog(site) {
		message, err := entry.Email.Message(to, site)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}

		page := message.HTML
		for _, image := range message.Inline {
			name := image.ContentID + ".png"
			if err := os.WriteFile(filepath.Join(out, name), image.Data, 0o644); err != nil {
				return err
			}
			page = strings.ReplaceAll(page, "cid:"+image.ContentID, name)
		}
		if err := os.WriteFile(filepath.Join(out, entry.ID+".html"), []byte(page), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, entry.ID+".txt"), []byte(message.Text), 0o644); err != nil {
			return err
		}

		if entry.Group != group {
			if group != "" {
				index.WriteString("</div>")
			}
			group = entry.Group
			index.WriteString("<h2>" + html.EscapeString(group) + "</h2><div class=\"grid\">")
		}
		var caption bytes.Buffer
		fmt.Fprintf(&caption, `<figure><figcaption><b>%s</b><span>%s</span><br><span>Subject: %s &middot; <a href="%s.html">HTML</a> &middot; <a href="%s.txt">text</a></span></figcaption><iframe src="%s.html" title="%s"></iframe></figure>`,
			html.EscapeString(entry.Title), html.EscapeString(entry.Trigger), html.EscapeString(message.Subject),
			entry.ID, entry.ID, entry.ID, html.EscapeString(entry.Title))
		index.Write(caption.Bytes())

		if sender != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := sender.Send(ctx, message)
			cancel()
			if err != nil {
				return fmt.Errorf("send %s: %w", entry.ID, err)
			}
		}
		count++
	}
	index.WriteString("</div></main>")
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte(index.String()), 0o644); err != nil {
		return err
	}

	fmt.Printf("Rendered %d emails to %s (open %s)\n", count, out, filepath.Join(out, "index.html"))
	if send {
		fmt.Printf("Sent %d emails to %s through %s:%d\n", count, to, config.Host, config.Port)
	}
	return nil
}
