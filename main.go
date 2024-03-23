package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	mail "github.com/xhit/go-simple-mail/v2"
)

var logger *slog.Logger

func main() {
	err := run()
	if err != nil {
		os.Exit(1)
	}
}

func run() (err error) {
	// Setup logging
	logFilename := fmt.Sprintf("backup-helper-%s.log", time.Now().Format(time.RFC3339))
	logFile, err := os.Create(logFilename)
	if err != nil {
		return fmt.Errorf("could not create log file %s: %w", logFilename, err)
	}
	defer logFile.Close()
	logger = slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, logFile), nil))

	// Log any error
	defer func() {
		if err != nil {
			logger.Error("program failed", "err", err.Error())
		}
	}()

	// Parse args
	args := os.Args[1:]
	if len(args) != 2 {
		return fmt.Errorf("expect exactly two args: first for input folder, second for output folder - but received %d", len(args))
	}
	inFolder, outFolder := args[0], args[1]
	logger.Debug("args parsed", "in", inFolder, "out", outFolder)

	// Load config
	err = loadConfig()
	if err != nil {
		return err
	}
	err = testMail()
	if err != nil {
		return err
	}

	// TODO:
	// - Check existence of .mounted files in source and target (smoke test)
	// - Do a writes test to source and target and check RW is working (smoke test)
	// - Run cshatag for both folders (in parallel?)
	// - Parse exit code and output to understand file events
	// - If failed to run, or any corrupt files: log, send an email, and stop (LES).
	// - Sync the source to the target using rsync
	// - Check for exit codes and capture output
	// - If sync failed: LES
	// - Then presuming all is well: LES
	return nil
}

type report struct {
	Title    string
	Detail   string
	Sections []section
}

type section struct {
	Title   string
	Detail  string
	Bullets []string
}

func sendMail(r report) error {
	wr := strings.Builder{}
	err := reportTmpl.Execute(&wr, r)
	if err != nil {
		return fmt.Errorf("could not template report: %w", err)
	}
	body := wr.String()

	email := mail.NewMSG().
		SetFrom(fmt.Sprintf("backup-helper <%s>", cfg.FromMail)).
		AddTo(cfg.ToMail).
		SetSubject(fmt.Sprintf("backup-helper %s", time.Now().Format(time.RFC3339))).
		SetBody(mail.TextHTML, body)
	if email.Error != nil {
		return fmt.Errorf("could not build email: %w", email.Error)
	}

	mailClient, err := mailClient()
	if err != nil {
		return err
	}
	defer mailClient.Close()

	err = email.Send(mailClient)
	if err != nil {
		return fmt.Errorf("could not send email: %w", err)
	}

	return nil
}

func mailClient() (*mail.SMTPClient, error) {
	mailSrv := mail.NewSMTPClient()
	mailSrv.Host = cfg.MailHost
	mailSrv.Port = cfg.MailPort
	mailSrv.Username = cfg.MailUser
	mailSrv.Password = cfg.MailPass
	switch cfg.MailEncryption {
	case "SSL/TLS":
		mailSrv.Encryption = mail.EncryptionSSLTLS
	case "STARTTLS":
		mailSrv.Encryption = mail.EncryptionSTARTTLS
	default:
		return nil, fmt.Errorf("unknown encryption in config: %q", cfg.MailEncryption)
	}

	mailClient, err := mailSrv.Connect()
	if err != nil {
		return nil, fmt.Errorf("could not connect to mail server: %w", err)
	}

	return mailClient, nil
}

var reportFmt = `<h2>{{.Title}}</h2>
<p>{{.Detail}}</p>

{{range .Sections}}
<h3>{{.Title}}</h3>

<p>{{.Detail}}</p>

<ul>
{{range .Bullets}}
<li>{{.}}</li>
{{end}}
</ul>

{{end}}
`
var reportTmpl = template.Must(template.New("report").Parse(reportFmt))

type config struct {
	// Matches json tags directly

	MailHost       string
	MailPort       int
	MailUser       string
	MailPass       string
	MailEncryption string

	FromMail string
	ToMail   string
}

var cfg *config

// Assumes config is in PWD as config.json. Sets on cfg global var.
func loadConfig() error {
	b, err := os.ReadFile("config.json")
	if err != nil {
		wd, _ := os.Getwd()
		return fmt.Errorf("could not read config.json in PWD (%s): %w", wd, err)
	}

	var c config
	err = json.Unmarshal(b, &c)
	if err != nil {
		wd, _ := os.Getwd()
		return fmt.Errorf("could not parse config.json in PWD (%s): %w", wd, err)
	}
	cfg = &c

	return nil
}

// ---
// --- Test/Debugging
// ---

func testMail() error {
	r := report{
		Title:  "Test mail",
		Detail: "This is the main introduction paragraph.",
		Sections: []section{
			{
				Title:  "Section 1",
				Detail: "This is the paragraph for section 1",
				Bullets: []string{
					"section 1 bullet 1",
					"section 1 bullet 2",
				},
			},
			{
				Title:  "Section 2",
				Detail: "This is the paragraph for section 2",
				Bullets: []string{
					"section 2 bullet 1",
					"section 2 bullet 2",
					"section 2 bullet 3",
				},
			},
		},
	}

	return sendMail(r)
}
