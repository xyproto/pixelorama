package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/xyproto/burnpal"
)

const (
	width  = 320
	height = 200
)

type MouseEvent struct {
	T string `json:"t"` // Minimized key for event type
	X int    `json:"x"`
	Y int    `json:"y"`
}

var (
	palette      color.Palette
	mu           sync.Mutex
	img          *image.Paletted
	clients      map[chan bool]bool
	lastSent     time.Time
	sendInterval = 50 * time.Millisecond
)

func init() {
	palette = burnpal.ColorPalette()
	img = image.NewPaletted(image.Rect(0, 0, width, height), palette)
	clients = make(map[chan bool]bool)
	lastSent = time.Now()
}

func main() {
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/pixels", servePixels)
	http.HandleFunc("/palette", servePalette)
	http.HandleFunc("/mouse_events", handleMouseEvents)
	http.HandleFunc("/events", handleEvents)

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	log.Println("Serving HTML")
	http.ServeFile(w, r, "index.html")
}

func servePixels(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Serving pixels")

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		log.Println("Error encoding PNG:", err)
		http.Error(w, "Error encoding image", http.StatusInternalServerError)
	}
}

func servePalette(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Serving palette")

	jsonPalette := make([]color.RGBA, len(palette))
	for i, c := range palette {
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		jsonPalette[i] = rgba
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jsonPalette); err != nil {
		log.Println("Error encoding palette:", err)
		http.Error(w, "Error encoding palette", http.StatusInternalServerError)
	}
}

func handleMouseEvents(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	var events []MouseEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		log.Println("Error decoding JSON:", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	for _, event := range events {
		log.Printf("Processing event: %s at (%d, %d)\n", event.T, event.X, event.Y)
		if event.X >= 0 && event.X < width && event.Y >= 0 && event.Y < height {
			red := color.RGBA{255, 0, 0, 255}
			img.Set(event.X, event.Y, red)
		}
	}

	if time.Since(lastSent) >= sendInterval {
		lastSent = time.Now()
		for c := range clients {
			c <- true
		}
	}
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling events")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	updateChan := make(chan bool)
	clients[updateChan] = true

	defer func() {
		delete(clients, updateChan)
		close(updateChan)
		log.Println("Client disconnected")
	}()

	for range updateChan {
		_, err := w.Write([]byte("data: update\n\n"))
		if err != nil {
			log.Println("Error writing to client:", err)
			break
		}
		w.(http.Flusher).Flush()
	}
}
