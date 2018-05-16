package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/17xande/keylogger"
	piplayer "github.com/17xande/pi-player/pkg/piPlayer"
)

func main() {
	addr := flag.String("addr", ":8080", "The addr of the application.")
	test := flag.String("test", "", "send \"mac\", \"linux\", or \"web\" to test the code on mac or linux or to test only the web interface.")
	debug := flag.Bool("debug", false, "print extra information for debugging.")
	flag.Parse()

	var conf piplayer.Config
	if err := conf.Load(""); err != nil {
		log.Fatal("Error loading config.", err)
	}

	dbg := *debug || conf.Debug

	if dbg {
		log.Println("Debug mode enabled")
		log.Println("Config file -> Directory: ", conf)
	}

	a := piplayer.NewAPIHandler(dbg, test)
	kl := keylogger.NewKeyLogger(conf.Remote.Name)
	p := piplayer.NewPlayer(&a, &conf, kl)

	defer p.CleanUp()

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("pkg/piPlayer/assets"))))
	http.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir(conf.Directory))))
	http.HandleFunc("/login", piplayer.LoginHandler(&conf))
	http.HandleFunc("/logout", piplayer.LogoutHandler)
	http.HandleFunc("/control", p.HandleControl)
	http.HandleFunc("/settings", conf.HandleSettings)
	http.HandleFunc("/viewer", p.HandleViewer)
	http.HandleFunc("/menu", p.HandleMenu)
	http.HandleFunc("/control/ws", p.Connection.HandlerWebsocket)
	http.HandleFunc("/api", a.Handle(p))
	http.HandleFunc("/", handlerHome)

	// Start the browser
	// We have to start it async because the code has
	// to carry on, so that the server comes online.
	go p.FirstRun()

	log.Printf("Listening on port %s\n", *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

// Handles requests to the index page as well as any other requests
// that don't match any other paths
func handlerHome(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path != "/" {
		http.Error(w, "Not found", 404)
		log.Printf("Not found: %s", r.URL)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		log.Printf("Method not allowed: %s", r.URL)
		return
	}
	_, loggedIn, err := piplayer.CheckLogin(w, r)
	if err != nil {
		log.Println("error trying to retrieve session on login page:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if loggedIn {
		http.Redirect(w, r, "/control", http.StatusFound)
	} else {
		http.Redirect(w, r, "/login", http.StatusFound)
	}

	t := piplayer.NewTemplateHandler("index.html")
	t.ServeHTTP(w, r)
}
