package cmd

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/32leaves/yamaha-openhab/pkg/musiccast"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "serves the API gateway",
	Run: func(cmd *cobra.Command, args []string) {
		devices, err := musiccast.Discover()
		if err != nil {
			log.WithError(err).Fatal("cannot discover devices")
		}
		if len(devices) == 0 {
			log.Fatal("no devices found")
		}

		dev := devices[0]
		log.WithField("name", dev.Name).WithField("url", dev.URL.String()).Info("device found")

		router := routes(&remoteDevice{dev})
		walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			log.Printf("%s %s\n", method, route)
			return nil
		}
		if err := chi.Walk(router, walkFunc); err != nil {
			log.WithError(err).Error("cannot list all routes")
		}

		addr, _ := cmd.Flags().GetString("address")
		log.Infof("server running on %s", addr)
		log.Fatal(http.ListenAndServe(addr, router))
	},
}

type remoteDevice struct {
	Origin *musiccast.Device
}

func (d *remoteDevice) ServeInfo(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.Sync()
	if err != nil {
		answerRequest(w, r, err)
		return
	}
	render.JSON(w, r, d.Origin)
}

func (d *remoteDevice) ServePowerOn(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.PowerOn()
	answerRequest(w, r, err)
}

func (d *remoteDevice) ServePowerOff(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.PowerOff()
	answerRequest(w, r, err)
}

type setVolumeRequest struct {
	Volume uint8 `json:"vol"`
}

func (sv *setVolumeRequest) Bind(r *http.Request) error {
	return nil
}

func (d *remoteDevice) ServeSetVolume(w http.ResponseWriter, r *http.Request) {
	data := &setVolumeRequest{}
	if err := render.Bind(r, data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.WithField("volume", data.Volume).Info("setting volume")
	err := d.Origin.SetVolume(data.Volume)
	answerRequest(w, r, err)
}

func (d *remoteDevice) ServeGetVolume(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, setVolumeRequest{d.Origin.Status.Volume})
}

func (d *remoteDevice) ServeMuteOn(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.SetMute(true)
	answerRequest(w, r, err)
}

func (d *remoteDevice) ServeMuteOff(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.SetMute(false)
	answerRequest(w, r, err)
}

func (d *remoteDevice) ServeAlbumArt(w http.ResponseWriter, r *http.Request) {
	err := d.Origin.Sync()
	if err != nil {
		answerRequest(w, r, err)
		return
	}

	arturl := d.Origin.GetAlbumArtURL()
	if arturl == "" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(arturl)
	if err != nil {
		answerRequest(w, r, err)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func answerRequest(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	render.JSON(w, r, "OK")
}

func routes(dev *remoteDevice) *chi.Mux {
	router := chi.NewRouter()
	router.Use(
		render.SetContentType(render.ContentTypeJSON), // Set content-Type headers as application/json
		middleware.Logger,          // Log API request calls
		middleware.DefaultCompress, // Compress results, mostly gzipping assets and json
		middleware.RedirectSlashes, // Redirect slashes to no slash URL versions
		middleware.Recoverer,       // Recover from panics without crashing server
	)

	router.Route("/v1", func(r chi.Router) {
		r.Get("/info", dev.ServeInfo)
		r.Post("/power/on", dev.ServePowerOn)
		r.Post("/power/off", dev.ServePowerOff)
		r.Post("/volume", dev.ServeSetVolume)
		r.Get("/volume", dev.ServeGetVolume)
		r.Post("/mute/on", dev.ServeMuteOn)
		r.Post("/mute/off", dev.ServeMuteOff)
		r.Get("/playback/album/art", dev.ServeAlbumArt)
	})

	return router
}

func init() {
	rootCmd.AddCommand(serveCmd)

	addr := os.Getenv("YAMAHA_OPENHAB_ADDR")
	if addr == "" {
		addr = "localhost:5000"
	}
	serveCmd.Flags().StringP("address", "a", addr, "address on which to listen")
}
