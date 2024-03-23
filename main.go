package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mail "github.com/xhit/go-simple-mail/v2"
)

var logWriter io.Writer
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
	logWriter = io.MultiWriter(os.Stderr, logFile)
	logger = slog.New(slog.NewTextHandler(logWriter, nil))

	// Log any error
	defer func() {
		if err != nil {
			logger.Error("program failed", "err", err.Error())
		}
	}()

	// Send an email at the end
	mailReport := report{
		Detail: fmt.Sprintf("Started at %s. This report includes info on the cshatag output, and the rsync output.",
			time.Now().Format(time.RFC3339)),
	}
	defer func() {
		if err != nil {
			mailReport.Title = "[ERROR] Backup Helper report"
			mailReport.Sections = append(mailReport.Sections, section{
				Title:  "Error",
				Detail: fmt.Sprintf("Error contents: %s", err.Error()),
			})
		} else {
			mailReport.Title = "[SUCCESS] Backup Helper report"
			mailReport.Sections = append(mailReport.Sections, section{
				Title:  "Success",
				Detail: "No error reported - looking good!",
			})
		}
		mErr := sendMail(mailReport)
		err = errors.Join(err, mErr)
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

	// Check folders
	err = checkFolder(inFolder)
	if err != nil {
		return fmt.Errorf("in folder: %w", err)
	}
	err = checkFolder(outFolder)
	if err != nil {
		return fmt.Errorf("out folder: %w", err)
	}
	mailReport.Sections = append(mailReport.Sections, section{
		Title:  "Folders checked",
		Detail: "This test tries to write, read, and delete a temporary file in both the input and output folders.",
		LogLines: []string{
			fmt.Sprintf("%s: OK", inFolder),
			fmt.Sprintf("%s: OK", outFolder),
		},
	})

	// Run cshatag for both folders (concurrently)
	logger.Debug("running cshatag on input and output folders (concurrently)")
	var wg sync.WaitGroup
	var cshaInErr, cshaOutErr error
	var cshaInLines, cshaOutLines []string
	wg.Add(2)
	go func() {
		defer wg.Done()
		cshaInLines, cshaInErr = execCommand("cshatag:input", "cshatag", "-q", "-recursive", inFolder)
		logger.Info("cshatag on input finished",
			"dir", inFolder,
			"lines", len(cshaInLines))
	}()
	go func() {
		defer wg.Done()
		cshaOutLines, cshaOutErr = execCommand("cshatag:output", "cshatag", "-q", "-recursive", outFolder)
		logger.Info("cshatag on output finished",
			"dir", outFolder,
			"lines", len(cshaOutLines))
	}()
	wg.Wait()
	addExecSection(&mailReport, "cshatag on input folder", cshaInLines,
		"cshatag", "-q", "-recursive", inFolder)
	addExecSection(&mailReport, "cshatag on output folder", cshaOutLines,
		"cshatag", "-q", "-recursive", outFolder)
	err = errors.Join(err, fmt.Errorf("cshatag on input folder failed: %w", cshaInErr))
	err = errors.Join(err, fmt.Errorf("cshatag on output folder failed: %w", cshaOutErr))
	if err != nil {
		return err
	}

	// Sync with rsync
	// -> Need a slash at the end of the in folder to indicate to rsync to sync the contents into out
	inWithSlash := inFolder + string(filepath.Separator)
	rsyncLines, err := execCommand("rsync", "rsync", "-avu", "--delete", inWithSlash, outFolder)
	addExecSection(&mailReport, "rsync from input to output folder", rsyncLines,
		"rsync", "-avu", "--delete", inWithSlash, outFolder)

	logger.Info("sync successful!")
	return nil
}

// Check the folders allow for read/write before doing anything
func checkFolder(dir string) error {
	testVal := rand.Int()
	filename := fmt.Sprintf("testfile-%d.txt", testVal)
	checkFile := filepath.Join(dir, filename)

	// Check write
	err := os.WriteFile(checkFile, []byte(strconv.Itoa(testVal)), 0644)
	if err != nil {
		return fmt.Errorf("write err: %w", err)
	}

	// Check read
	readBytes, err := os.ReadFile(checkFile)
	if err != nil {
		return fmt.Errorf("read err: %w", err)
	}
	readVal, _ := strconv.Atoi(string(readBytes))
	if testVal != readVal {
		return fmt.Errorf("read check failed: different value (wanted %d, got %s)", testVal, string(readBytes))
	}

	// Cleanup
	err = os.Remove(checkFile)
	if err != nil {
		return fmt.Errorf("cleanup err: %w", err)
	}

	logger.Info("folder check passed", "dir", dir)
	return nil
}

func execCommand(
	logDesc string,
	name string,
	args ...string,
) (lines []string, err error) {
	// Write program output both to logs and to a buffer
	linew := linesWriter{}
	logw := lineBuffer{
		Out:    logWriter,
		Prefix: []byte(fmt.Sprintf("[%s] ", logDesc)),
	}
	wr := io.MultiWriter(&logw, &linew)

	logger.Debug("executing command",
		"command", name,
		"args", args)
	cmd := exec.Command(name, args...)
	cmd.Stdout = wr
	cmd.Stderr = wr

	err = cmd.Run()
	logw.Flush()
	lines = linew.Lines()
	lines = append(lines, "<end of logs>")
	if err != nil {
		return lines, fmt.Errorf("command %s failed: %w", name, err)
	}

	return lines, nil
}

type report struct {
	Title    string
	Detail   string
	Sections []section
}

type section struct {
	Title    string
	Detail   string
	LogLines []string
}

func addExecSection(r *report, desc string, outLines []string, name string, args ...string) {
	r.Sections = append(r.Sections, section{
		Title:    desc,
		Detail:   fmt.Sprintf("[%s %s]", name, strings.Join(args, " ")),
		LogLines: outLines,
	})
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
		SetSubject(r.Title).
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

	logger.Info("mail sent",
		"to", cfg.ToMail,
		"subject", r.Title)
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

var reportFmt = `
<h2>{{.Title}}</h2>
<p>{{.Detail}}</p>

{{range .Sections}}
<h3>{{.Title}}</h3>

{{if .Detail}}<p>{{.Detail}}</p>{{end}}

{{if .LogLines}}
<pre style="font-family: monospace; font-size: 10px; line-height: 12px; background-color: #b5b5b5;"><code>
{{range .LogLines}}
{{.}}
{{end}}
</code></pre>
{{end}}

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
