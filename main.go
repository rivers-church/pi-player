package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sync"
)

func main() {
	addr := flag.String("addr", ":8080", "The addr of the application.")
	test := flag.Bool("test", false, "use macOs quicklook to test on a Mac")
	debug := flag.Bool("debug", false, "print extra information for debugging")
	flag.Parse()

	if *debug {
		log.Println("Debug mode enabled")
	}

	data, err := ioutil.ReadFile("config.json")
	var conf config
	if err != nil {
		log.Fatal("Error reading config file: ", err)
	}

	err = json.Unmarshal(data, &conf)
	if err != nil {
		log.Fatal("Error unmarshalling config file: ", err)
	}

	if *debug {
		log.Println("Config file -> Directory: ", conf)
	}

	a := apiHandler{debug: *debug, test: *test}
	p := Player{api: &a, conf: conf}

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))
	http.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir(conf.Directory))))
	http.HandleFunc("/control", p.handleControl)
	http.HandleFunc("/viewer", p.handleViewer)
	http.HandleFunc("/control/ws", p.control.handlerWebsocket)
	http.HandleFunc("/api", a.handle(&p))
	http.HandleFunc("/", handlerHome)
	// http.HandleFunc("/command", handlerCommand(&p))

	log.Printf("Listening on port %s\n", *addr)
	err = http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

type config struct {
	Directory string
}

type templateHandler struct {
	once     sync.Once
	filename string
	templ    *template.Template
	data     map[string]interface{}
}

// ServeHTTP handles HTTP requests for the templates
func (t *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// once keeps track of which of these anonymous functions have already been called,
	// and stores their result. If they are called again it just returns the stored result.
	// t.once.Do(func(){
	t.templ = template.Must(template.ParseFiles(filepath.Join("templates", t.filename)))
	// // })
	// data := map[string]string{
	// 	"Host": r.Host,
	// }

	t.templ.Execute(w, t.data)
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

	t := templateHandler{filename: "index.html"}
	t.ServeHTTP(w, r)
}

type apiHandler struct {
	debug   bool
	test    bool
	message reqMessage
}

// handle handles all calls to the API
func (a *apiHandler) handle(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// handle POST requests
		if r.Method != "POST" {
			m := &resMessage{Success: false, Message: "Invalid request method: " + r.Method}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		// handle only application/json requests
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			m := &resMessage{Success: false, Message: "Invalid Content-Type: " + ct}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		// decode message
		a.message = reqMessage{}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&a.message)
		defer r.Body.Close()
		if err != nil {
			m := &resMessage{Success: false, Message: "Error decoding JSON request: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		if a.debug {
			log.Println("message received: ", a.message)
		}

		// displach execution based on which component was called
		// in this case, the Player component
		if a.message.Component == "player" {
			p.ServeHTTP(w, r)
			return
		}

		// return a generic success message for debugging
		m := &resMessage{
			Success: true,
			Message: fmt.Sprintf("Message Received:\ncomponent: %s\nmethod: %s\narguments: %v\n", a.message.Component, a.message.Method, a.message.Arguments),
		}
		json.NewEncoder(w).Encode(m)
		log.Println(m.Message)
	}
}
