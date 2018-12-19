package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"image"
	"image/draw"
	"image/jpeg"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	mux := http.NewServeMux()
	files := http.FileServer(http.Dir("public"))
	mux.Handle("/static/", http.StripPrefix("/static/", files))
	mux.HandleFunc("/", upload)
	mux.HandleFunc("/mosaic", mosaic)
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	// building up the source tile database
	tilesDB()
	fmt.Println("Mosaic server started.")
	server.ListenAndServe()
}

func upload(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("upload.html")
	t.Execute(w, nil)
}

func mosaic(w http.ResponseWriter, r *http.Request) {
	t0 := time.Now()

	// get the content from the POSTed form
	r.ParseMultipartForm(10485760) // max body in memory is 10MB
	file, _, _ := r.FormFile("image")
	defer file.Close()
	tileSize, _ := strconv.Atoi(r.FormValue("tile_size"))

	// decode and get original image
	original, _, _ := image.Decode(file)
	bounds := original.Bounds()

	// create a new image for the mosaic
	newImage := image.NewNRGBA(image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y))

	// Clone tile database
	db := cloneTilesDB()

	singleThreadedMosiac(&db, original, newImage, &bounds, tileSize)

	buf1 := new(bytes.Buffer)
	jpeg.Encode(buf1, original, nil)
	originalStr := base64.StdEncoding.EncodeToString(buf1.Bytes())

	buf2 := new(bytes.Buffer)
	jpeg.Encode(buf2, newImage, nil)
	mosaic := base64.StdEncoding.EncodeToString(buf2.Bytes())
	t1 := time.Now()
	images := map[string]string{
		"original": originalStr,
		"mosaic":   mosaic,
		"duration": fmt.Sprintf("%v", t1.Sub(t0)),
	}

	t, _ := template.ParseFiles("results.html")
	t.Execute(w, images)
}

func singleThreadedMosiac(
	db *map[string][3]float64, original image.Image, newImage *image.NRGBA, bounds *image.Rectangle, tileSize int) {
	// source point for each tile, which starts with 0, 0 of each tile
	sp := image.Point{0, 0}
	for y := bounds.Min.Y; y < bounds.Max.Y; y = y + tileSize {
		for x := bounds.Min.X; x < bounds.Max.X; x = x + tileSize {
			// use the top left most pixel as the average color
			r, g, b, _ := original.At(x, y).RGBA()
			color := [3]float64{float64(r), float64(g), float64(b)}

			// get the closest tile from the tiles DB
			nearest := nearest(color, db)
			file, err := os.Open(nearest)
			if err != nil {
				fmt.Println("error:", nearest)
				continue
			}

			img, _, err := image.Decode(file)
			if err != nil {
				fmt.Println("error:", err, nearest)
				continue
			}

			// resize the tile to the correct size
			t := resize(img, tileSize)
			tile := t.SubImage(t.Bounds())
			tileBounds := image.Rect(x, y, x+tileSize, y+tileSize)

			// draw the tile into the mosaic
			draw.Draw(newImage, tileBounds, tile, sp, draw.Src)
		}
	}
}
