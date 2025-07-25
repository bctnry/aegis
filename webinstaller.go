package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bctnry/aegis/pkg/aegis"
	dbinit "github.com/bctnry/aegis/pkg/aegis/db/init"
	rsinit "github.com/bctnry/aegis/pkg/aegis/receipt/init"
	ssinit "github.com/bctnry/aegis/pkg/aegis/session/init"
	"github.com/bctnry/aegis/templates"
)

type WebInstallerRoutingContext struct {
	Template *template.Template
	// yes, we do share the same object between multiple goroutine,
	// but i don't think this would be a problem for a simple web
	// installer.
	// step 1 - plain mode or non-plain mode?
	//          use namespace or not?
	//          plain mode - goto step [6]
	// step 2 - database config
	// step 3 - session config
	// step 4 - mailer config
	// step 5 - receipt system config
	// step 6 - git root & git user
	// step 7 - ignored namespaces & repositories
	// step 8 - web front setup:
	//          depot name
	//          front page config
	//          (static assets dir default to be $HOME/aegis-static/)
	//          bind address & port
	//          http host name
	//          ssh host name (disabled if plain mode)
	//          allow registration
	//          email confirmation required
	//          manual approval
	// plain mode on: 1-6-7-8
	// plain mode off: 1-2-3-4-5-6-8
	Step int
	Config *aegis.AegisConfig
	ConfirmStageReached bool
	ResultingFilePath string
}

func logTemplateError(e error) {
	if e != nil { log.Print(e) }
}

func (ctx *WebInstallerRoutingContext) loadTemplate(name string) *template.Template {
	return ctx.Template.Lookup(name)
}

func withLog(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf(" %s %s\n", r.Method, r.URL.Path)
		f(w, r)
	}
}

func foundAt(w http.ResponseWriter, p string) {
	w.Header().Add("Content-Length", "0")
	w.Header().Add("Location", p)
	w.WriteHeader(302)
}

func (ctx *WebInstallerRoutingContext) reportRedirect(target string, timeout int, title string, message string, w http.ResponseWriter) {
	logTemplateError(ctx.loadTemplate("webinstaller/_redirect").Execute(w, templates.WebInstRedirectWithMessageModel{
		Timeout: timeout,
		RedirectUrl: target,
		MessageTitle: title,
		MessageText: message,
	}))
}

func bindAllWebInstallerRoutes(ctx *WebInstallerRoutingContext) {
	http.HandleFunc("GET /", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/start").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	
	http.HandleFunc("GET /step1", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step1").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step1", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step1", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.PlainMode = len(r.Form.Get("plain-mode")) > 0
		ctx.Config.UseNamespace = len(r.Form.Get("enable-namespace")) > 0
		if ctx.Config.PlainMode {
			foundAt(w, "/step6")
		} else {
			foundAt(w, "/step2")
		}
	}))
	
	http.HandleFunc("GET /step2", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step2").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step2", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step2", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Database = aegis.AegisDatabaseConfig{
			Type: strings.TrimSpace(r.Form.Get("database-type")),
			Path: strings.TrimSpace(r.Form.Get("database-path")),
			URL: strings.TrimSpace(r.Form.Get("database-url")),
			UserName: strings.TrimSpace(r.Form.Get("database-username")),
			DatabaseName: strings.TrimSpace(r.Form.Get("database-database-name")),
			TablePrefix: strings.TrimSpace(r.Form.Get("database-table-prefix")),
			Password: strings.TrimSpace(r.Form.Get("database-password")),
		}

 		foundAt(w, "/step3")
	}))
	
	http.HandleFunc("GET /step3", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step3").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step3", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step3", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("session-database-number")), 10, 64)
		if err != nil {
			ctx.reportRedirect("/step3", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Session = aegis.AegisSessionConfig{
			Type: strings.TrimSpace(r.Form.Get("session-type")),
			Path: strings.TrimSpace(r.Form.Get("session-path")),
			TablePrefix: strings.TrimSpace(r.Form.Get("session-table-prefix")),
			Host: strings.TrimSpace(r.Form.Get("session-host")),
			UserName: strings.TrimSpace(r.Form.Get("session-username")),
			Password: strings.TrimSpace(r.Form.Get("session-password")),
			DatabaseNumber: int(i),
		}
		foundAt(w, "/step4")
	}))

	
	http.HandleFunc("GET /step4", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step4").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step4", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step4", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("mailer-smtp-port")), 10, 64)
		if err != nil {
			ctx.reportRedirect("/step4", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.Mailer = aegis.AegisMailerConfig{
			Type: strings.TrimSpace(r.Form.Get("mailer-type")),
			SMTPServer: strings.TrimSpace(r.Form.Get("mailer-smtp-server")),
			SMTPPort: int(i),
			User: strings.TrimSpace(r.Form.Get("mailer-user")),
			Password: strings.TrimSpace(r.Form.Get("mailer-password")),
		}
		foundAt(w, "/step5")
	}))
	
	http.HandleFunc("GET /step5", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step5").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step5", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step5", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.ReceiptSystem = aegis.AegisReceiptSystemConfig{
			Type: strings.TrimSpace(r.Form.Get("receipt-system-type")),
			Path: strings.TrimSpace(r.Form.Get("receipt-system-path")),
			URL: strings.TrimSpace(r.Form.Get("receipt-system-url")),
			UserName: strings.TrimSpace(r.Form.Get("receipt-system-username")),
			DatabaseName: strings.TrimSpace(r.Form.Get("receipt-system-database-name")),
			Password: strings.TrimSpace(r.Form.Get("receipt-system-password")),
			TablePrefix: strings.TrimSpace(r.Form.Get("receipt-system-table-prefix")),
		}
		foundAt(w, "/step6")
	}))
	
	http.HandleFunc("GET /step6", withLog(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(ctx.Config.GitUser, ctx.Config.GitRoot)
		logTemplateError(ctx.loadTemplate("webinstaller/step6").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step6", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step6", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.GitRoot = strings.TrimSpace(r.Form.Get("git-root"))
		setUserName := strings.TrimSpace(r.Form.Get("git-user"))
		var u *user.User
		if len(setUserName) <= 0 {
			u, _ = user.Current()
		} else {
			u, err = user.Lookup(setUserName)
			if err != nil {
				ctx.reportRedirect("/step6", 0, "No User", fmt.Sprintf("The user name you've provided seems to not usable due to reason: %s. Please try again or use a different user.", err.Error()), w)
				return
			}
		}
		ctx.Config.GitUser = u.Username
		ctx.Config.StaticAssetDirectory = path.Join(u.HomeDir, "aegis-static")
		next := ""
		if ctx.Config.PlainMode {
			next = "/step7"
		} else {
			next = "/step8"
		}
		err = templates.UnpackStaticFileTo(ctx.Config.StaticAssetDirectory)
		if err != nil {
			ctx.reportRedirect(next, 0, "Failed", fmt.Sprintf("Static file unpack is unsuccessful due to reason: %s. You can still move forward but would have to unpack static file yourself.", err.Error()), w)
			return
		}
		foundAt(w, next)
	}))
	
	http.HandleFunc("GET /step7", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step7").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step7", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step1", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.IgnoreNamespace = make([]string, 0)
		for k := range strings.SplitSeq(r.Form.Get("ignore-namespace"), ",") {
			ctx.Config.IgnoreNamespace = append(ctx.Config.IgnoreNamespace, k)
		}
		ctx.Config.IgnoreRepository = make([]string, 0)
		for k := range strings.SplitSeq(r.Form.Get("ignore-repository"), ",") {
			ctx.Config.IgnoreRepository = append(ctx.Config.IgnoreRepository, k)
		}
		foundAt(w, "/step8")
	}))
	
	http.HandleFunc("GET /step8", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/step8").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /step8", withLog(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			ctx.reportRedirect("/step1", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.DepotName = strings.TrimSpace(r.Form.Get("depot-name"))
		ctx.Config.BindAddress = strings.TrimSpace(r.Form.Get("bind-address"))
		i, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("bind-port")), 10, 64)
		if err != nil {
			ctx.reportRedirect("/step8", 0, "Invalid Request", "The request is of an invalid form. Please try again.", w)
			return
		}
		ctx.Config.BindPort = int(i)
		ctx.Config.HttpHostName = strings.TrimSpace(r.Form.Get("http-host-name"))
		ctx.Config.SshHostName = strings.TrimSpace(r.Form.Get("ssh-host-name"))
		frontPageType := strings.TrimSpace(r.Form.Get("front-page-type"))
		switch frontPageType {
		case "all/namespace": fallthrough
		case "all/repository":
			ctx.Config.FrontPageType = frontPageType
		case "static/html": fallthrough
		case "static/text": fallthrough
		case "static/markdown": fallthrough
		case "static/org":
			ctx.Config.FrontPageType = frontPageType
			ctx.Config.FrontPageContent = r.Form.Get("front-page-text")
		case "repository":
			v := r.Form.Get("front-page-value")
			ctx.Config.FrontPageType = "repository/" + v
		case "namespace":
			v := r.Form.Get("front-page-value")
			ctx.Config.FrontPageType = "namespace/" + v
		}
		ctx.Config.AllowRegistration = len(strings.TrimSpace(r.Form.Get("allow-registration"))) > 0
		ctx.Config.EmailConfirmationRequired = len(strings.TrimSpace(r.Form.Get("email-confirmation-required"))) > 0
		ctx.Config.ManualApproval = len(strings.TrimSpace(r.Form.Get("manual-approval"))) > 0
		foundAt(w, "/confirm")
	}))
	
	http.HandleFunc("GET /confirm", withLog(func(w http.ResponseWriter, r *http.Request) {
		ctx.ConfirmStageReached = true
		logTemplateError(ctx.loadTemplate("webinstaller/confirm").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
			ConfirmStageReached: ctx.ConfirmStageReached,
		}))
	}))
	http.HandleFunc("POST /confirm", withLog(func(w http.ResponseWriter, r *http.Request) {

		pwusr, err := user.Lookup(ctx.Config.GitUser)
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed to retrieve info about the specified Git user %s. Please fix this and restart the web installer.", ctx.Config.GitUser),
				w,
			)
			return
		} else if pwusr == nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Cannot find user %s. Please fix this and restart the web installer.", ctx.Config.GitUser),
				w,
			)
			return
		}
		p := path.Join(pwusr.HomeDir, fmt.Sprintf("aegis-config-%d.json", time.Now().Unix()))
		ctx.Config.Version = 0
		ctx.Config.FilePath = p
		err = ctx.Config.Sync()
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed to save config to %s: %s. Please fix this and restart the web installer.", p, err.Error()),
				w,
			)
			return
		}
		
		ctx.Config.RecalculateProperPath()
		
		dbif, err := dbinit.InitializeDatabase(ctx.Config)
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to initialize database for setup: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		dbifVerdict, err := dbif.IsDatabaseUsable()
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to check the database: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		if !dbifVerdict {
			err = dbif.InstallTables()
			if err != nil {
				ctx.reportRedirect("/", 0, "Failure",
					fmt.Sprintf("Failed while trying to set up the database: %s. Please fix this and restart the web installer.", err.Error()),
					w,
				)
				return
			}
		}
		dbif.Dispose()
		
		ssif, err := ssinit.InitializeDatabase(ctx.Config)
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to initialize session store for setup: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		ssifVerdict, err := ssif.IsSessionStoreUsable()
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to check the session store: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		if !ssifVerdict {
			err = ssif.Install()
			if err != nil {
				ctx.reportRedirect("/", 0, "Failure",
					fmt.Sprintf("Failed while trying to set up the session store: %s. Please fix this and restart the web installer.", err.Error()),
					w,
				)
				return
			}
		}
		ssif.Dispose()
		
		rsif, err := rsinit.InitializeReceiptSystem(ctx.Config)
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to initialize receipt system for setup: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		rsifVerdict, err := rsif.IsReceiptSystemUsable()
		if err != nil {
			ctx.reportRedirect("/", 0, "Failure",
				fmt.Sprintf("Failed while trying to check the receipt system: %s. Please fix this and restart the web installer.", err.Error()),
				w,
			)
			return
		}
		if !rsifVerdict {
			err = rsif.Install()
			if err != nil {
				ctx.reportRedirect("/", 0, "Failure",
					fmt.Sprintf("Failed while trying to set up the receipt system: %s. Please fix this and restart the web installer.", err.Error()),
					w,
				)
				return
			}
		}
		rsif.Dispose()

		foundAt(w, "/finish")
	}))
	
	http.HandleFunc("GET /finish", withLog(func(w http.ResponseWriter, r *http.Request) {
		logTemplateError(ctx.loadTemplate("webinstaller/finish").Execute(w, &templates.WebInstallerTemplateModel{
			Config: ctx.Config,
		}))
	}))
}

func WebInstaller() {
	fmt.Println("This is the Aegis web installer. We will start a web server, which allows us to provide you a more user-friendly interface for configuring your Aegis instance. This web server will be shut down when the installation is finished. You can always start the web installer by using the `-init` flag or the `install` command.")
	var portNum int = 0
	for {
		r, err := askString("Please enter the port number this web server would bind to.", "8001")
		if err != nil {
			fmt.Printf("Failed to get a response: %s\n", err.Error())
			os.Exit(1)
		}
		portNum, err = strconv.Atoi(strings.TrimSpace(r))
		if err == nil { break }
		fmt.Println("Please enter a valid number...")
	}
	masterTemplate := templates.LoadTemplate()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%d", portNum),
	}
	bindAllWebInstallerRoutes(&WebInstallerRoutingContext{
		Template: masterTemplate,
		Config: &aegis.AegisConfig{},
	})
	go func() {
		log.Printf("Trying to serve at %s:%d\n", "0.0.0.0", portNum)
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	<-sigChan
	
	if err := server.Shutdown(context.TODO()); err != nil {
		log.Fatalf("HTTP shutdown fail: %v", err)
	}
	
	log.Println("Graceful shutdown complete.")
}

