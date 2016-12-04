// TODO : Make install.sh with specific versions

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/Compufreak345/alice"
	"github.com/Compufreak345/manners"
	"github.com/julienschmidt/httprouter"
	pretty "github.com/tonnerre/golang-pretty"

	"github.com/OpenDriversLog/webfw"
	"github.com/OpenDriversLog/goodl-lib/translate"

	"github.com/Compufreak345/dbg"
	"github.com/OpenDriversLog/goodl-lib/tools"

	conf "github.com/OpenDriversLog/goodl/config"
	"github.com/OpenDriversLog/goodl/utils/userManager"

	adrMan "github.com/OpenDriversLog/goodl/controllers/addressManager"
	betaMan "github.com/OpenDriversLog/goodl/controllers/betaMan"
	carMan "github.com/OpenDriversLog/goodl/controllers/carManager"
	colorMan "github.com/OpenDriversLog/goodl/controllers/colorManager"
	deviceMan "github.com/OpenDriversLog/goodl/controllers/deviceManager"
	driverMan "github.com/OpenDriversLog/goodl/controllers/driverManager"
	notificationMan "github.com/OpenDriversLog/goodl/controllers/notificationManager"
	errorCont "github.com/OpenDriversLog/goodl/controllers/error"
	export "github.com/OpenDriversLog/goodl/controllers/export"
	inv "github.com/OpenDriversLog/goodl/controllers/invite"
	jsonApi "github.com/OpenDriversLog/goodl/controllers/jsonApi"
	odl "github.com/OpenDriversLog/goodl/controllers/odl"
	reg "github.com/OpenDriversLog/goodl/controllers/register"
	resetPassword "github.com/OpenDriversLog/goodl/controllers/resetPassword"
	sendMail "github.com/OpenDriversLog/goodl/controllers/sendMail"
	signupBeta "github.com/OpenDriversLog/goodl/controllers/signupBeta"
	static "github.com/OpenDriversLog/goodl/controllers/staticContent"
	sync "github.com/OpenDriversLog/goodl/controllers/syncDB"
	syncMan "github.com/OpenDriversLog/goodl/controllers/syncMan"
	trips "github.com/OpenDriversLog/goodl/controllers/trips"
	tutorialMan "github.com/OpenDriversLog/goodl/controllers/tutorialManager"
	userMan "github.com/OpenDriversLog/goodl/controllers/userManager"
	keyMan "github.com/OpenDriversLog/goodl/controllers/keyManager"
	upload "github.com/OpenDriversLog/goodl/controllers/upload"


	"github.com/OpenDriversLog/goodl/utils/notificationChecker"
)

// t is the default translater for the backend.
var t translate.Translater
// config is the config of the webserver.
var config *webfw.ServerConfig

const gTag = dbg.Tag("goodl/goodl.go")

// init initializes, mainly getting the default config.
func init() {
	config = conf.GetConfig()
}

// router is used for routing HTTP-requests.
var router = httprouter.New()

// Vulcanize determines if pages should be vulcanized before serving them (only active when not in development mode)
var Vulcanize = true

// Minify determines if pages should be minified before serving them - only works together with vulcanize!
// (only active when not in development mode)
var Minify = true

// main starts the ODL-webserver, creates shortened translation files & starts notification worker.
func main() {
	dbg.I(gTag, "Server starting up with config : \n %# v\n", pretty.Formatter(config))

	for _, v := range os.Args[1:] {
		if v == "novulcanize" {
			Vulcanize = false
			Minify = false
			dbg.I(gTag, "Vulcanization disabled by command line arg.")
		}
	}
	tools.RegisterSqlite("SQLITE")

	t = translate.Translater{
		DefaultLang:  "de-DE",
		FallbackLang: "en-US",
	}
	var err error
	dbg.D(gTag, "Removing linebreaks from javascript translation files...")
	var fs []os.FileInfo
	fs, err = ioutil.ReadDir(config.RootDir + "/public/translations")
	if err != nil {
		dbg.E(gTag, "Error scanning for files in /public/translations ", err)
	}

	os.MkdirAll(config.RootDir+"/public/translations-autogenerated", os.FileMode(755))

	if dbg.Develop { // check for changes in translations every second if we are developing
		go func() {
			defer func() {
				if err := recover(); err != nil {
					dbg.E(gTag, "Error refreshing translations:", err)
				}
			}()
			lastTimes := make(map[string]time.Time)

			for {
				time.Sleep(1 * time.Second)
				for _, f := range fs {
					if strings.HasSuffix(f.Name(), ".json") {
						path := config.RootDir + "/public/translations/" + f.Name()
						newPath := config.RootDir + "/public/translations-autogenerated/" + f.Name()
						a, _ := os.Stat(path)

						if lastTimes[path].Before(a.ModTime()) {
							dbg.I(gTag, "Removing line breaks from ", path, lastTimes[path], a.ModTime())
							err = removeLineBreaks(path, newPath)

						}
						if err != nil {
							dbg.E(gTag, "Error removing linebreaks for %s : ", f, err)
						} else {
							lastTimes[path] = a.ModTime()
						}
					}
				}
			}
		}()
	} else { // load translations only once in production.
		for _, f := range fs {
			if strings.HasSuffix(f.Name(), ".json") {
				path := config.RootDir + "/public/translations/" + f.Name()
				err = removeLineBreaks(path, config.RootDir+"/public/translations-autogenerated/"+f.Name())

				if err != nil {
					dbg.E(gTag, "Error removing linebreaks for ", err)
				}
			}
		}
	}

	// Handle translations...
	dbg.D(gTag, "Removing linebreaks from golang translation files...")
	err = removeLineBreaks(config.RootDir+"/translations/de-DE.all.json", config.RootDir+"/translations/de-DE.all.autogenerated.json")
	if err != nil {
		dbg.E(gTag, "Error removing linebreaks for de-DE - stop it! ", err)
	}
	err = removeLineBreaks(config.RootDir+"/translations/en-US.all.json", config.RootDir+"/translations/en-US.all.autogenerated.json")
	if err != nil {
		dbg.E(gTag, "Error removing linebreaks for en-US - stop it! ", err)
	}
	dbg.D(gTag, "Loading translation files...")
	t.MustLoadTranslationFile(config.RootDir + "/translations/de-DE.all.autogenerated.json")
	t.MustLoadTranslationFile(config.RootDir + "/translations/en-US.all.autogenerated.json")
	//config.T = t
	// configure webfw....
	config.TimeoutMessage = t.T("requestTimeout")
	webfw.SetConfig(config)
	webfw.SetDefaultTranslater(&t)
	err = userManager.UpdateDbsIfNeeded(config.SharedDir + "/userDb.db")
	if err != nil {
		dbg.E(gTag, "Fatal error while updating DB - shutting down : %v", err)
		return
	}
	webfw.MyErrorController = errorCont.ErrorController{}
	webfw.ErrorViewDataPolishFunc = ViewDataPolishFunc

	// Register handlers
	commonHandlers := alice.New(webfw.RecoverHandler,
		webfw.LoggingHandler, webfw.GorillaClearHandler)
	sessionSecuredHandlers := alice.New(webfw.RecoverHandler,
		webfw.LoggingHandler, webfw.GorillaClearHandler, userManager.LoginWallHandler)

	// Manages adresses
	webfw.MVCBinders["adrMan"] = webfw.MVCBinder{Ctrl: adrMan.AddressManagerController{}}
	// Manages beta-invites
	webfw.MVCBinders["betaMan"] = webfw.MVCBinder{Ctrl: betaMan.BetaManController{}}
	// Manages cars
	webfw.MVCBinders["carMan"] = webfw.MVCBinder{Ctrl: carMan.CarManagerController{}}
	// Manages colors
	webfw.MVCBinders["colorMan"] = webfw.MVCBinder{Ctrl: colorMan.ColorManagerController{}}
	// provides CRUD for synchronisations, as well as triggering an update of synchronized data.
	webfw.MVCBinders["syncMan"] = webfw.MVCBinder{Ctrl: syncMan.SyncManController{}}
	// Manages drivers
	webfw.MVCBinders["driverMan"] = webfw.MVCBinder{Ctrl: driverMan.DriverManagerController{}}
	// Manages devices
	webfw.MVCBinders["deviceMan"] = webfw.MVCBinder{Ctrl: deviceMan.DeviceManagerController{}}
	// Responsible for PDF-export
	webfw.MVCBinders["export"] = webfw.MVCBinder{Ctrl: export.ExportController{}}
	// Manages invite-keys.
	webfw.MVCBinders["invite"] = webfw.MVCBinder{Ctrl: inv.InviteController{}}
	// Serves the main-page
	webfw.MVCBinders["odl"] = webfw.MVCBinder{Ctrl: odl.OdlController{}}
	// provides several JSON-methods for managing devices, notifications, tracks & trips
	webfw.MVCBinders["jsonApi"] = webfw.MVCBinder{Ctrl: jsonApi.JsonApiController{}}
	// Manages device-keys, for automatic upload.
	webfw.MVCBinders["keyMan"] = webfw.MVCBinder{Ctrl: keyMan.KeyManController{}}
	// Manages notifications (reminders)
	webfw.MVCBinders["notificationMan"] = webfw.MVCBinder{Ctrl: notificationMan.NotificationManagerController{}}
	// Manages the registration process
	webfw.MVCBinders["register"] = webfw.MVCBinder{Ctrl: reg.RegisterController{}}
	// Responsible for resetting passwords
	webfw.MVCBinders["resetPassword"] = webfw.MVCBinder{Ctrl: resetPassword.ResetPasswordController{}}
	// Responsible for sending support-mails & creating issues
	webfw.MVCBinders["SendMail"] = webfw.MVCBinder{Ctrl: sendMail.SendMailController{}}
	// Responsible for applying for the beta-test
	webfw.MVCBinders["signupBeta"] = webfw.MVCBinder{Ctrl: signupBeta.SignupBetaController{}}
	// Servers static pages in the views/static-folder
	webfw.MVCBinders["static"] = webfw.MVCBinder{Ctrl: static.StaticContentController{}}
	// Responsible for uploading trips to a users database, e.g. from a CSV or KML
	webfw.MVCBinders["SyncDB"] = webfw.MVCBinder{Ctrl: sync.SyncDBController{}}
	// Manages trips
	webfw.MVCBinders["Trips"] = webfw.MVCBinder{Ctrl: trips.TripsController{}}
	// Responsible for saving the tutorial state of users.
	webfw.MVCBinders["tutorialMan"] = webfw.MVCBinder{Ctrl: tutorialMan.TutorialManagerController{}}
	// Responsible for managing the automatic upload process.
	webfw.MVCBinders["upload"] = webfw.MVCBinder{Ctrl: upload.UploadController{}}

	vulcanizedItems = make(map[string]string)
	os.MkdirAll(config.RootDir+"/views/vulcanized/minified", 0755)
	// Wrap the binders above with a translation-handler.
	HandleLanguage("odl/*carbonrouterpath", commonHandlers.ThenContext(webfw.GetMvcHandler("odl", ViewDataPolishFunc)), 0)

	HandleLanguage("signupBeta", commonHandlers.ThenContext(webfw.GetMvcHandler("signupBeta", nil)), 0)

	HandleLanguage("register", commonHandlers.ThenContext(webfw.GetMvcHandler("register", ViewDataPolishFunc)), 0)
	HandleLanguage("resetPassword", commonHandlers.ThenContext(webfw.GetMvcHandler("resetPassword", ViewDataPolishFunc)), 0)
	HandleLanguage("handleLogin", commonHandlers.ThenFuncContext(userManager.HandleLoginHandler), 0)
	HandleLanguage("initEnc", commonHandlers.ThenFuncContext(userManager.InitEncryptionHandler), 0)
	HandleLanguage("userMan", commonHandlers.ThenFuncContext(userMan.HandleUserManRequest), 0)
	HandleLanguage("upload", commonHandlers.ThenContext(webfw.GetMvcHandler("upload", ViewDataPolishFunc)), time.Duration(20*time.Minute))

	// session secured handlers
	HandleStaticLanguage("static", commonHandlers.ThenContext(webfw.GetMvcHandler("static", ViewDataPolishFunc)))

	HandleLanguage("adrMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("adrMan", ViewDataPolishFunc)), 0)
	HandleLanguage("betaMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("betaMan", ViewDataPolishFunc)), 0)
	HandleLanguage("carMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("carMan", ViewDataPolishFunc)), 0)
	HandleLanguage("clearCache", sessionSecuredHandlers.ThenFuncContext(webfw.ClearCacheHandler), 0)
	HandleLanguage("colorMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("colorMan", ViewDataPolishFunc)), 0)
	HandleLanguage("syncMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("syncMan", ViewDataPolishFunc)), time.Duration(20*time.Minute))
	HandleLanguage("deviceMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("deviceMan", ViewDataPolishFunc)), 0)
	HandleLanguage("driverMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("driverMan", ViewDataPolishFunc)), 0)
	HandleLanguage("export", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("export", ViewDataPolishFunc)), 0)
	HandleLanguage("index.html", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("Welcome", ViewDataPolishFunc)), 0)
	HandleLanguage("invite", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("invite", ViewDataPolishFunc)), 0)
	HandleLanguage("jsonApi", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("jsonApi", ViewDataPolishFunc)), 0)
	HandleLanguage("keyMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("keyMan", ViewDataPolishFunc)), 0)

	HandleLanguage("logout", sessionSecuredHandlers.ThenFuncContext(userManager.LogoutHandler), 0)
	HandleLanguage("notificationMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("notificationMan", ViewDataPolishFunc)), 0)
	HandleLanguage("protectedDownload/*filepath", sessionSecuredHandlers.ThenFuncContext(ProtectedContentHandler), 0)
	HandleLanguage("sendMail", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("SendMail", ViewDataPolishFunc)), 0)
	HandleLanguage("syncDB", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("SyncDB", nil)), 3*time.Hour)
	HandleLanguage("trips", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("Trips", ViewDataPolishFunc)), 0)
	HandleLanguage("tutorialMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("tutorialMan", ViewDataPolishFunc)), 0)

	HandleLanguage("usrMan", sessionSecuredHandlers.ThenContext(webfw.GetMvcHandler("usrMan", ViewDataPolishFunc)), 0)

	// Serve files out of the public-folder
	router.GET(config.SubDir+"/:lang/public/*filepath", ProvideFolderContentHandler)

	// 404 page
	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webfw.DirectShowError_NoVD(nil, w, r, nil, http.StatusText(404), 404, true)
	})

	// Handler for the worst case - should never occur!
	router.PanicHandler = func(w http.ResponseWriter, r *http.Request, err interface{}) {
		dbg.WTF(gTag, "PANIC fell through up to router - that is totally not wanted!")
		webfw.DirectShowError_NoVD(nil, w, r, nil, http.StatusText(500), 500)
	}
	working := make(chan bool,1)

	// Now start the server!
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt, os.Kill)
		<-sigchan
		working <- true
		dbg.I(gTag, "Shutting down...")
		manners.Close()
	}()
	go func() {
		for {
			dbg.I(gTag,"Starting checking for overdue notifications")
			working <- true
			err := notificationChecker.CheckForOverDue()
			if err != nil {
				dbg.E(gTag,"Error checking for overdue notifications : ", err)
			}
			<-working
			dbg.I(gTag,"Finished checking for overdue notifications")
			time.Sleep(10*time.Minute)
		}
	}()
	manners.ListenAndServe(config.HttpAddress, router)
}

// removeLineBreaks removes linebreaks from the .json-files for them to be easier to maintain and still provide valid JSON
func removeLineBreaks(sourceFName string, targetFName string) (err error) {
	input, err := ioutil.ReadFile(sourceFName)
	if err != nil {
		return
	}

	lines := strings.Split(string(input), "\n")
	output := strings.Join(lines, " ")
	err = ioutil.WriteFile(targetFName, []byte(output), 0644)

	return
}

// HandleLanguage returns a function to be used with httprouter that puts a function for localization into the context-object.
func HandleLanguage(path string, handleFunc alice.CtxHandler, customTimeout time.Duration) {
	fn := func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer func() {
			if err := recover(); err != nil {
				dbg.E(gTag, "panic in HandleLanguage: %v for request : %v", err, dbg.GetRequest(r))
				webfw.DirectShowError(webfw.ViewData{ErrorType: 500}, errors.New(fmt.Sprintf("%s", err)), w)
			}
		}()
		if dbg.Develop {
			dbg.D(gTag, "My url : %s ", r.RequestURI)
		} else {
			dbg.D(gTag, "My url : %s ", r.URL.Path)
		}
		lang := ps.ByName("lang")
		serveFn := func(ch chan struct{}, ctx context.Context) {
			ctx = context.WithValue(ctx, "T", webfw.GetTranslater(lang))
			handleFunc.ServeHTTP(ctx, w, r)
			ch <- struct{}{}

		}
		webfw.WrapWithInit(serveFn, w, customTimeout)

	}
	dbg.I(gTag, "HandleLang Bind %s", path)
	router.GET(config.SubDir+"/:lang/"+path, fn)
	router.POST(config.SubDir+"/:lang/"+path, fn)
}

// HandleStaticLanguage returns a function to be used with httprouter that puts a function for localization into the context-object
// AND adds the :staticView: value to the context.
func HandleStaticLanguage(path string, handleFunc alice.CtxHandler) {
	fn := func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer func() {
			if err := recover(); err != nil {
				dbg.E(gTag, "panic in HandleLanguage: %v for request : %v", err, dbg.GetRequest(r))
				webfw.DirectShowError(webfw.ViewData{ErrorType: 500}, errors.New(fmt.Sprintf("%s", err)), w)

			}
		}()
		if dbg.Develop {
			dbg.D(gTag, "My url : %s ", r.RequestURI)
		} else {
			dbg.D(gTag, "My url : %s ", r.URL.Path)
		}
		lang := ps.ByName("lang")
		staticView := ps.ByName("staticView")
		serveFn := func(ch chan struct{}, ctx context.Context) {
			ctx = context.WithValue(ctx, "T", webfw.GetTranslater(lang))
			ctx = context.WithValue(ctx, "staticView", staticView)
			handleFunc.ServeHTTP(ctx, w, r)
			ch <- struct{}{}

		}
		webfw.WrapWithInit(serveFn, w, 0)

	}
	dbg.I(gTag, "Binding %s", path)
	router.GET(config.SubDir+"/:lang/"+path+"/*staticView", fn)
}


var vulcanizedItems map[string]string

// ViewDataPolishFunc adds translations and user data to the view data,
// gets called after every view call to set global assets,
// caches the vulcanized results if dbg.Develop is not true
func ViewDataPolishFunc(vd *webfw.ViewData, ctx context.Context, r *http.Request, vPath string) (newViewPath string) {
	newViewPath = vPath
	defer func() {
		if err := recover(); err != nil {
			dbg.E(gTag, "Error in ViewDataPolishFunc", err)
			vd.ErrorType = 500
		}
	}()

	if Vulcanize {
		var err error
		if vulcanizedItems["Polymer"] == "" {
			if Minify && !dbg.Develop {
				_, err = minifyItem("public/components/polymer/polymer.html")
				if err != nil {
					dbg.E(gTag, "Error minifying item %s : ", "public/components/polymer/polymer_vulcanized.html", err)
				}
				_, err = minifyItem("public/components/polymer/polymer-micro.html")
				if err != nil {
					dbg.E(gTag, "Error minifying item %s : ", "public/components/polymer/polymer_micro.html", err)
				}
				_, err = minifyItem("public/components/polymer/polymer-mini.html")
				if err != nil {
					dbg.E(gTag, "Error minifying item %s : ", "public/components/polymer/polymer_mini.html", err)
				}
				vulcanizedItems["Polymer"] = "Done"
			}
		}
		if vulcanizedItems[vPath] == "" || dbg.Develop {
			if strings.HasPrefix(vPath, "views") {
				newViewPath, err = vulcanizeItem(vPath)
				if err != nil {
					dbg.E(gTag, "Error vulcanizing item %s : ", vPath, err)
				}
				if Minify && !dbg.Develop {
					newViewPath, err = minifyItem(newViewPath)
					if err != nil {
						dbg.E(gTag, "Error minifying item %s : ", vPath, err)
					}
				}
			}
			vulcanizedItems[vPath] = newViewPath
		} else {
			newViewPath = vulcanizedItems[vPath]
		}
	}

	if vd.T == nil {
		var T *translate.Translater
		if ctx != nil {
			if ctx.Value("T") != nil {
				T = ctx.Value("T").(*translate.Translater)
			}
		}
		if T == nil {
			dbg.W(gTag, "No Translater found in ViewDataPolishFunc - fallback to default, but this is not good.")
			T = webfw.DefaultTranslater()
		}
		vd.T = T
	}
	dbg.V(gTag, "I am at ViewDataPolishFunc")

	if vd.Data == nil {
		vd.Data = make(map[string]interface{})
	}
	usr, _, _ := userManager.GetUserWithSession(r)
	if usr == nil || !usr.IsLoggedIn() {
		vd.Data["UserLevel"] = -1
	} else {
		vd.Data["UserLevel"] = usr.Level()
	}
	if vd.Data["User"] == nil {

		if usr == nil || !usr.IsLoggedIn() {
			vd.Data["User"] = nil
		} else {
			uData := make(map[string]interface{})
			uData["LoginName"] = usr.LoginName()
			uData["FirstName"] = usr.FirstName()
			uData["LastName"] = usr.LastName()
			uData["Title"] = usr.Title()
			uData["Id"] = usr.Id()
			uData["Level"] = usr.Level()
			uData["TutorialDisabled"] = usr.TutorialDisabled()
			uData["NextNotificationTime"] = usr.NextNotificationTime()
			uData["NotificationsEnabled"] = usr.NotificationsEnabled()

			vd.Data["User"] = uData
		}
	}

	translations := vd.T.GetTranslationsForView(vd.ViewName)

	marshaled, err := json.Marshal(translations)

	if err != nil {
		dbg.E(gTag, "Error while marshaling JSON translation in ViewDataPolishFunc", err)
		vd.ErrorType = 500
		return
	}
	vd.Data["Translations"] = template.JS(marshaled)
	timeLoad := time.Now().UnixNano()
	vd.Data["TimeLoad"] = timeLoad
	dbg.I(gTag, "Loaded view %s with timeLoad of %s", vd.ViewName, timeLoad)
	// If setting last param to true, error messages from request will not be appended to error messages that were added in go.
	webfw.AddMessagesToViewData(vd, r, false)
	dbg.V(gTag, "Finished ViewDataPolishFunc")
	return
}

// vulcanizeItem vulcanizes the given view.
func vulcanizeItem(viewPath string) (newViewPath string, err error) {
	dbg.I(gTag, "Start vulcanize ", viewPath)
	newViewPath = viewPath
	name := path.Base(viewPath)
	nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))

	notVulcanized := config.RootDir + "/public/bundledImports/" + nameNoExt + ".html"
	isPolymer := strings.Contains(viewPath, "public/components/polymer/")

	if isPolymer {
		notVulcanized = config.RootDir + "/" + viewPath
	}
	var importExists = true
	if _, err = os.Stat(notVulcanized); os.IsNotExist(err) {
		importExists = false
	} else if err != nil {
		dbg.E(gTag, "Error checking if file exists %s : ", notVulcanized, err)
		return
	}

	if importExists {
		outpath := config.RootDir + "/public/bundledImports/" + nameNoExt + "_vulcanized.html"
		if isPolymer {
			outpath = strings.Replace(viewPath, ".html", "_vulcanized.html", 1)
		}
		cmd := exec.Command("vulcanize", "--exclude", "./public/components/webcomponentsjs/webcomponents-lite.js", "--exclude", "./public/components/webcomponentsjs/webcomponents-lite.min.js", "--exclude", "./public/components/webcomponentsjs/webcomponents.js", "--exclude", "./public/components/polymer/polymer.html", notVulcanized)
		cmd.Dir = config.RootDir
		var outfile *os.File
		// open the out file for writing
		outfile, err = os.Create(outpath)
		if err != nil {
			panic(err)
		}
		defer outfile.Close()
		cmd.Stdout = outfile

		err = cmd.Start()
		if err != nil {
			dbg.E(gTag, "Error starting vulcanize command : ", err)
			return
		}
		err = cmd.Wait()
		if err != nil {
			dbg.E(gTag, "Error waiting for vulcanize command : ", err)
			return
		}
	}
	if viewPath == "views/layout.html" || isPolymer { // Layout needs no replacements.
		return
	}

	// Replace path in file

	newViewPath = "views/vulcanized/" + nameNoExt + ".html"
	fsPath := config.RootDir + "/" + viewPath
	if _, err = os.Stat(fsPath); os.IsNotExist(err) {
		return
	} else if err != nil {
		dbg.E(gTag, "Error checking if file exists %s : ", fsPath, err)
		return
	}
	input, err := ioutil.ReadFile(fsPath)
	if err != nil {
		dbg.E(gTag, "Error reading view file : ", err)
		return
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		if strings.Contains(line, "public/bundledImports/"+nameNoExt) {
			lines[i] = strings.Replace(line, "public/bundledImports/"+nameNoExt, "public/bundledImports/"+nameNoExt+"_vulcanized", 1)

		} else if strings.Contains(line, "public/bundledImports/layout.html") {
			lines[i] = strings.Replace(line, "public/bundledImports/layout", "public/bundledImports/layout"+"_vulcanized", 1)

		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(config.RootDir+"/"+newViewPath, []byte(output), 0644)
	if err != nil {
		dbg.E(gTag, "Error writing view file : ", err)
		return
	}
	dbg.I(gTag, "Finish vulcanize ", viewPath)
	return
}

// minifyItem minifies the given view. Needs to be vulcanized before minimizing.
func minifyItem(viewPath string) (newViewPath string, err error) {
	dbg.I(gTag, "Start minify ", viewPath)
	newViewPath = viewPath
	name := path.Base(viewPath)
	nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))
	isPolymer := strings.Contains(viewPath, "public/components/polymer/")

	notMinified := config.RootDir + "/public/bundledImports/" + nameNoExt + "_vulcanized.html"
	if isPolymer {
		notMinified = config.RootDir + "/" + viewPath
	}
	var importExists = true
	if _, err = os.Stat(notMinified); os.IsNotExist(err) {
		importExists = false
	} else if err != nil {
		dbg.E(gTag, "Error checking if file exists %s : ", notMinified, err)
		return
	}
	outpath := "--out=" + config.RootDir + "/public/bundledImports/" + nameNoExt + "_vulcanized_minified.html"

	if importExists {
		if isPolymer {
			outpath = "--out=" + config.RootDir + "/" + viewPath
		}

		cmd := exec.Command("grunt", "minifyPolymer", "--in="+notMinified, outpath)
		cmd.Dir = config.RootDir
		err = cmd.Start()
		if err != nil {
			dbg.E(gTag, "Error starting minify command : ", err)
			return
		}
		err = cmd.Wait()
		if err != nil {
			dbg.E(gTag, "Error waiting for minify command : ", err)
			return
		}
		if viewPath == "views/vulcanized/layout.html" || isPolymer { // Layout needs no replacements.
			return
		}
	}

	// Replace path in file

	newViewPath = "views/vulcanized/minified/" + nameNoExt + ".html"

	fsPath := config.RootDir + "/" + viewPath
	if _, err = os.Stat(fsPath); os.IsNotExist(err) {
		return
	} else if err != nil {
		dbg.E(gTag, "Error checking if file exists %s : ", fsPath, err)
		return
	}
	input, err := ioutil.ReadFile(fsPath)
	if err != nil {
		dbg.E(gTag, "Error reading view file : ", err)
		return
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		if strings.Contains(line, "public/bundledImports/"+nameNoExt) {
			lines[i] = strings.Replace(line, "public/bundledImports/"+nameNoExt+"_vulcanized", "public/bundledImports/"+nameNoExt+"_vulcanized_minified", 1)
		} else if strings.Contains(line, "public/bundledImports/layout_vulcanized.html") {
			lines[i] = strings.Replace(line, "public/bundledImports/layout_vulcanized", "public/bundledImports/layout_vulcanized_minified", 1)
		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(config.RootDir+"/"+newViewPath, []byte(output), 0644)
	if err != nil {
		dbg.E(gTag, "Error writing view file : ", err)
		return
	}
	dbg.I(gTag, "Finished minify ", viewPath)
	return
}

/*
 ProtectedContentHandler is used to provide content inside the user-specific exported folder (databases/upload/userId/exported)
*/
func ProtectedContentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	usr, _, _ := userManager.GetUserWithSession(r)
	if usr == nil || !usr.IsLoggedIn() {
		webfw.DirectShowError_NoVD(ctx, w, r, errors.New("Restricted"), "Internal server error", 500, true)
		dbg.WTF(gTag, "This is impossible. How did I get to GetProtectedContentHandler without logging in?")
	}
	folderAbsolute := userManager.GetUserWorkDir(usr.Id()) + "/exported/"
	T := ctx.Value("T").(*translate.Translater)
	partToRemoveFromUrlPath := T.UrlLang + "/protectedDownload/exported"
	err := webfw.ProvideFolderContentHandler(ctx, w, r, "", partToRemoveFromUrlPath, true, folderAbsolute)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			dbg.I(gTag, "File not found : ", err)
			webfw.DirectShowError_NoVD(nil, w, r, nil, http.StatusText(404), 404, true)
		} else {
			dbg.E(gTag, "panic in ProvideFolderContentHandler: %v for request : %v", err, dbg.GetRequest(r))
			webfw.DirectShowError(webfw.ViewData{ErrorType: 500, NoStyleOnError: true}, err, w)
		}
	}
}
// ProvideFolderContentHandler serves the contents of the public-folder.
func ProvideFolderContentHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	defer func() {
		if err := recover(); err != nil {
			var eString = fmt.Sprintf("%s", err)
			if strings.Contains(eString, "no such file or directory") {
				dbg.I(gTag, "File not found : ", err)
				webfw.DirectShowError_NoVD(nil, w, r, nil, http.StatusText(404), 404)
			} else {
				dbg.E(gTag, "panic in ProvideFolderContentHandler: %v for request : %v", err, dbg.GetRequest(r))
				webfw.DirectShowError(webfw.ViewData{ErrorType: 500}, errors.New(eString), w)
			}

		}
	}()
	serveFn := func(ch chan struct{}, ctx context.Context) {
		err := webfw.ProvideFolderContentHandler(ctx, w, r, "public", ps.ByName("lang")+"/public/", false, "")
		if err != nil {
			if strings.Contains(err.Error(), "no such file or directory") {
				dbg.I(gTag, "File not found : ", err)
				webfw.DirectShowError_NoVD(nil, w, r, nil, http.StatusText(404), 404)
			} else {
				dbg.E(gTag, "panic in ProvideFolderContentHandler: %v for request : %v", err, dbg.GetRequest(r))
				webfw.DirectShowError(webfw.ViewData{ErrorType: 500}, err, w)
			}
		}
		ch <- struct{}{}
	}
	webfw.WrapWithInit(serveFn, w, 0)

}
