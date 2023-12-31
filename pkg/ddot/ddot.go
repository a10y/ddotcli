// Package ddot connects to Washington DC DDOT's publicly accessible MQTT server instance.
package ddot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"slices"
	"strconv"
	"sync"
)

const MQTTServerUrl = "wss://b-8c165eea-0974-40be-9e62-ad394d480541-1.mq.us-east-1.amazonaws.com:61619//"

func CreateRandomClientID() string {
	var randBytes [8]byte
	_, err := rand.Read(randBytes[:])
	if err != nil {
		panic(fmt.Errorf("error: failed to create 8 bytes of randomness: %v", err))
	}

	return fmt.Sprintf("client-%s", hex.EncodeToString(randBytes[:]))
}

// Create a DDOT Client

type Client interface {
	GetCameras() []CameraInfo
	GetFfmpegUrl(cameraId string) string
}

type CameraInfo struct {
	Id           string
	Name         string
	Latitude     float32
	Longitude    float32
	HLSStreamUrl string
}

type DDOTTopic string

const (
	TopicCamera DDOTTopic = "DDOT/Camera"
)

// ddotClient is a simple in-memory system for keeping track of DDOT CCTV cameras
type ddotClient struct {
	client  mqtt.Client
	lock    sync.RWMutex
	cameras []CameraInfo
}

func (d *ddotClient) GetFfmpegUrl(cameraId string) string {
	idx := slices.IndexFunc(d.cameras, func(info CameraInfo) bool {
		return info.Id == cameraId
	})
	if idx < 0 {
		log.Fatalf("Failed to find camera %v", cameraId)
	}

	return d.cameras[idx].HLSStreamUrl
}

func (d *ddotClient) GetCameras() []CameraInfo {
	d.lock.RLock()
	defer d.lock.RUnlock()

	return d.cameras[:]
}

// ddotClient implements Client
var _ Client = (*ddotClient)(nil)

// Figure out the message types for processing the list of available cameras at the current time.
// Simple Go program for creating a bunch of cameras, browsing them, and setting them up.
// On a change in the setup of cameras, we want to reset our available list. We also want to show a new set of sensors.

type rawCameraMessage struct {
	AgencyId        string `json:"agencyId"`
	Checksum        string `json:"checksum"`
	DateCreated     string `json:"dateCreated"`
	Host            string `json:"host"`
	Id              string `json:"id"`
	Lat             string `json:"lat"`
	Lng             string `json:"lng"`
	MapDataSourceId string `json:"mapDataSourceId"`
	Status          string `json:"status"`
	Stream          string `json:"stream"`
	Title           string `json:"title"`
}

func init() {
	// Figure out how we're going to retrieve all of these values instead here.
}

func CreateClient() (Client, error) {
	// Setup logging
	//mqtt.ERROR = log.New(os.Stdout, "[ERROR] ", 0)
	//mqtt.CRITICAL = log.New(os.Stdout, "[CRIT] ", 0)
	//mqtt.WARN = log.New(os.Stdout, "[WARN]  ", 0)
	//mqtt.DEBUG = log.New(os.Stdout, "[DEBUG] ", 0)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(MQTTServerUrl)
	clientID := CreateRandomClientID()
	opts.SetClientID(clientID)
	opts.SetUsername("dcdot")
	opts.SetPassword("cctvddotpublic")

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.Wait() {
		return nil, fmt.Errorf("failed connecting to broker")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("failed connecting to broker: %v", err)
	}

	var cameras []CameraInfo
	ddot := &ddotClient{
		client:  client,
		cameras: cameras,
	}

	client.Subscribe(string(TopicCamera), 0, func(_ mqtt.Client, msg mqtt.Message) {
		ddot.lock.Lock()
		defer ddot.lock.Unlock()

		// msg
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Fatalf("error: failed to deserialize camera message: %v", err)
		}

		for k, v := range payload {
			if _, err := strconv.Atoi(k); err != nil {
				continue
			}

			// Load the messages from here.
			var cameraMsg rawCameraMessage
			data, _ := json.Marshal(v)
			_ = json.Unmarshal(data, &cameraMsg)

			latf32, err := strconv.ParseFloat(cameraMsg.Lat, 32)
			if err != nil {
				log.Fatalf("error parsing latitude from %v: %v", cameraMsg.Lat, err)
			}

			lonf32, err := strconv.ParseFloat(cameraMsg.Lng, 32)
			if err != nil {
				log.Fatalf("error parsing longitude from %v: %v", cameraMsg.Lng, err)
			}

			ddot.cameras = append(ddot.cameras, CameraInfo{
				Id:        cameraMsg.Id,
				Name:      cameraMsg.Title,
				Latitude:  float32(latf32),
				Longitude: float32(lonf32),
				HLSStreamUrl: fmt.Sprintf("https://%v/rtplive/%v/playlist.m3u8",
					cameraMsg.Host,
					cameraMsg.Stream),
			})
		}
	})
	return ddot, nil
}
