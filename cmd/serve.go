package cmd

import (
	"net/http"

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
		log.WithField("name", dev.Name).Info("device found")

		router := routes(&remoteDevice{dev})
		walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			log.Printf("%s %s\n", method, route)
			return nil
		}
		if err := chi.Walk(router, walkFunc); err != nil {
			log.WithError(err).Error("cannot list all routes")
		}

		log.Info("server running on :8080")
		log.Fatal(http.ListenAndServe(":8080", router))
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
	})

	return router
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
