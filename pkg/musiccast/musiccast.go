package musiccast

// based on https://github.com/almightycouch/couchpotatoe/blob/master/musiccast/musiccast.go

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"sync"

	"github.com/cskr/pubsub"
	upnp "github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/av1"
)

type event map[string]interface{}

type Status struct {
	Input     string `json:"input"`
	Power     string `json:"power"`
	Sleep     uint8  `json:"sleep"`
	Volume    uint8  `json:"volume"`
	Mute      bool   `json:"mute"`
	MaxVolume uint8  `json:"max_volume"`
}

type Playback struct {
	Input       string `json:"input"`
	Playback    string `json:"playback"`
	Repeat      string `json:"repeat"`
	Shuffle     string `json:"shuffle"`
	PlayTime    int32  `json:"play_time"`
	TotalTime   int32  `json:"total_time"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AlbumArtURL string `json:"albumart_url"`
	Track       string `json:"track"`
}

type Device struct {
	ID       string   `json:"id"`
	Model    string   `json:"model"`
	Name     string   `json:"name"`
	Status   Status   `json:"status"`
	Playback Playback `json:"playback"`

	URL         url.URL
	httpClient  *http.Client
	avTransport *av1.AVTransport1
	mutex       sync.RWMutex
}

var broker = pubsub.New(1)

// Discover attempts to find MusicCast devices on the local network.
func Discover() (devices []*Device, err error) {
	maybeRootDevices, err := upnp.DiscoverDevices("urn:schemas-upnp-org:device:MediaRenderer:1")
	if err == nil {
		for _, maybeRoot := range maybeRootDevices {
			d, err := NewDevice(maybeRoot)
			if err != nil {
				return nil, err
			}

			devices = append(devices, d)
		}
	}

	return devices, err
}

// NewDevice creates a new Device from the given UPnP root device.
func NewDevice(maybeRoot upnp.MaybeRootDevice) (device *Device, err error) {
	err = maybeRoot.Err
	if err == nil {
		ExtendedControlURL := maybeRoot.Root.Device.PresentationURL.URL
		ExtendedControlURL.Path = path.Join(ExtendedControlURL.Path, "YamahaExtendedControl", "v1")
		avTransportClients, err := av1.NewAVTransport1ClientsFromRootDevice(maybeRoot.Root, maybeRoot.Location)
		if err != nil {
			return nil, err
		}

		device = &Device{
			httpClient:  &http.Client{},
			URL:         ExtendedControlURL,
			avTransport: avTransportClients[0],
		}
		err = device.Sync()
		if err != nil {
			return nil, err
		}
	}

	return device, nil
}

// Play begins playback of the current track.
func (d *Device) Play() (err error) {
	return d.setPlayback("play")
}

// Pause pauses playback of the current track.
func (d *Device) Pause() (err error) {
	return d.setPlayback("pause")
}

// TogglePlayPause toggles playback state from "play" to "pause" and vice versa.
func (d *Device) TogglePlayPause() (err error) {
	return d.setPlayback("play_pause")
}

// Next plays the next track.
func (d *Device) Next() (err error) {
	return d.setPlayback("next")
}

// Previous plays the previous track.
func (d *Device) Previous() (err error) {
	return d.setPlayback("previous")
}

// PowerOn turns the device on
func (d *Device) PowerOn() (err error) {
	resp, err := d.requestWithParams("GET", "main/setPower", map[string]interface{}{
		"power": "on",
	})
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// GetAlbumArtURL returns the URL to the current album art
func (d *Device) GetAlbumArtURL() string {
	if d.Playback.AlbumArtURL == "" {
		return ""
	}

	u := url.URL{
		Scheme: d.URL.Scheme,
		Host:   d.URL.Host,
		Path:   d.Playback.AlbumArtURL,
	}
	return u.String()
}

// PowerOff turns the device off
func (d *Device) PowerOff() (err error) {
	resp, err := d.requestWithParams("GET", "main/setPower", map[string]interface{}{
		"power": "standby",
	})
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// SetVolume sets the volume to the given value.
func (d *Device) SetVolume(volume uint8) (err error) {
	params := map[string]interface{}{"volume": volume}
	resp, err := d.requestWithParams("GET", "main/setVolume", params)
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// IncreaseVolume increases the volume by the given value.
func (d *Device) IncreaseVolume(step uint8) (err error) {
	params := map[string]interface{}{"volume": "up", "step": step}
	resp, err := d.requestWithParams("GET", "main/setVolume", params)
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// DecreaseVolume decreases the volume by the given value.
func (d *Device) DecreaseVolume(step uint8) (err error) {
	params := map[string]interface{}{"volume": "down", "step": step}
	resp, err := d.requestWithParams("GET", "main/setVolume", params)
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// SetMute mutes and unmutes the volume.
func (d *Device) SetMute(mute bool) (err error) {
	params := map[string]interface{}{"enable": mute}
	resp, err := d.requestWithParams("GET", "main/setMute", params)
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

// Subscribe returns a channel for receiving update notifications from the device.
func (d *Device) Subscribe() chan interface{} {
	return broker.Sub(d.ID)
}

func (d *Device) fetchDeviceInfo() (err error) {
	resp, err := d.request("GET", "system/getDeviceInfo")
	if err != nil {
		return nil
	}

	data, err := decodeResponse(resp)
	if err != nil {
		return err
	}

	d.ID = data["device_id"].(string)
	d.Model = data["model_name"].(string)
	return nil
}

func (d *Device) fetchNetworkStatus() (err error) {
	resp, err := d.request("GET", "system/getNetworkStatus")
	if err != nil {
		return nil
	}

	data, err := decodeResponse(resp)
	if err != nil {
		return nil
	}

	d.Name = data["network_name"].(string)
	return nil
}

func (d *Device) fetchStatus() (err error) {
	resp, err := d.request("GET", "main/getStatus")
	if err == nil {
		defer resp.Body.Close()
		err = json.NewDecoder(resp.Body).Decode(&d.Status)
	}

	return err
}

func (d *Device) fetchPlayback() (err error) {
	resp, err := d.request("GET", "netusb/getPlayInfo")
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&d.Playback)
	if err != nil {
		return err
	}

	return nil
}

func (d *Device) Sync() (err error) {
	err = d.fetchDeviceInfo()
	if err != nil {
		return err
	}

	err = d.fetchNetworkStatus()
	if err != nil {
		return err
	}

	err = d.fetchStatus()
	if err != nil {
		return err
	}

	err = d.fetchPlayback()
	if err != nil {
		return err
	}

	return nil
}

func (d *Device) setPlayback(playback string) (err error) {
	params := map[string]interface{}{"playback": playback}
	resp, err := d.requestWithParams("GET", "netusb/setPlayback", params)
	if err == nil {
		_, err = decodeResponse(resp)
	}

	return err
}

func (d *Device) request(m string, p string) (resp *http.Response, err error) {
	return d.requestWithParams(m, p, make(map[string]interface{}))
}

func (d *Device) requestWithParams(m string, p string, q map[string]interface{}) (resp *http.Response, err error) {
	url := d.URL
	url.Path = path.Join(url.Path, p)

	req, err := http.NewRequest(m, url.String(), nil)
	if err == nil {
		req.Header.Add("X-AppName", "MusicCast/1.50")
		req.Header.Add("X-AppPort", "41100")
		if len(q) > 0 {
			params := req.URL.Query()
			for k, v := range q {
				params.Add(k, fmt.Sprint(v))
			}
			req.URL.RawQuery = params.Encode()
		}
		resp, err = d.httpClient.Do(req)
	}

	return resp, err
}

func decodeResponse(resp *http.Response) (data map[string]interface{}, err error) {
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err == nil {
		respCode := data["response_code"].(float64)
		delete(data, "response_code")
		if respCode != 0 {
			err = fmt.Errorf("extended control error %f", respCode)
		}
	}
	return data, err
}

func diffState(av, bv reflect.Value) interface{} {
	at := av.Type()
	switch kind := at.Kind(); kind {
	case reflect.Bool:
		if a, b := av.Bool(), bv.Bool(); a != b {
			return b
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a, b := av.Int(), bv.Int(); a != b {
			return b
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if a, b := av.Uint(), bv.Uint(); a != b {
			return b
		}
	case reflect.Float32, reflect.Float64:
		if a, b := av.Float(), bv.Float(); a != b {
			return b
		}
	case reflect.Complex64, reflect.Complex128:
		if a, b := av.Complex(), bv.Complex(); a != b {
			return b
		}
	case reflect.String:
		if a, b := av.String(), bv.String(); a != b {
			return b
		}
	case reflect.Interface:
		if v := diffState(av.Elem(), bv.Elem()); v != nil {
			return bv.Interface()
		}
	case reflect.Ptr:
		break
	case reflect.Struct:
		d := make(event)
		for i := 0; i < av.NumField(); i++ {
			if v := diffState(av.Field(i), bv.Field(i)); v != nil {
				if k := at.Field(i).Tag.Get("json"); k != "" {
					d[k] = v
				}
			}
		}
		if len(d) > 0 {
			return d
		}
	default:
		panic("unknown reflect Kind: " + kind.String())
	}

	return nil
}

func updateIn(field interface{}, update map[string]interface{}) (err error) {
	if len(update) > 0 {
		data, err := json.Marshal(update)
		if err == nil {
			err = json.Unmarshal(data, field)
		}
	}

	return err
}
