package controller

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/cisco-gve/tviewer/model"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gopkg.in/mgo.v2"
)

const ifSubscriptionID = "tviewerIFCS"
const isisSubscriptionID = "tviewerISIS"
const ifSensorGroupID = "tviewerInterfaces"
const isisSensorGroupID = "tviewerISISNeighbor"
const sampleInterval = 2000

var (
	indexController    index
	homeController     home
	topologyController topology
	devicesController  devices
	// BasePath stores the reference path for other controllers to look for files
	BasePath string
)

// Startup initialates the server controllers
func Startup(templates map[string]*template.Template, r *mux.Router) {

	// Create cert directory if doesn't exist
	_ = os.Mkdir(BasePath+"/certs", os.ModePerm)

	// Create the channel
	telemetryChan := make(chan model.TelemetryWrapper)

	indexController.indexTemplate = templates["index.html"]
	indexController.registerRoutes(r)

	homeController.homeTemplate = templates["home.html"]
	homeController.registerRoutes(r)

	devicesController.devicesTemplate = templates["devices.html"]
	devicesController.telemetryChannel = telemetryChan
	devicesController.registerRoutes(r)

	topologyController.topologyTemplate = templates["topology.html"]
	topologyController.clients = make(map[*websocket.Conn]bool)
	topologyController.broadcast = make(chan model.Topology)
	topologyController.wsUpgrader = websocket.Upgrader{}
	topologyController.registerRoutes(r)

	// Start telemetry of devices that are in the database
	// Open database
	session, err := mgo.Dial(os.Getenv("TELEMETRY_DB"))
	if err != nil {
		log.Fatal("Cannot open database:" + err.Error() + "\n")
	}
	defer session.Close()

	// Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	dbCollection := session.DB("Telemetry").C("Devices")

	var devices []model.Device

	err = dbCollection.Find(nil).All(&devices)
	if err != nil {
		log.Fatal("Cannot read devices table:" + err.Error() + "\n")
	}
	for _, device := range devices {
		n := Node{}
		n.Ip = device.Ip
		n.CertName = BasePath + "/certs/" + device.Name + ".pem"
		n.Name = device.Name
		n.Username = device.Username
		n.Password = device.Password
		n.Port = device.Port

		go n.CollectInterfaceData(telemetryChan)
		go n.CollectISISData(telemetryChan)
		go n.watchForOldData(telemetryChan)
	}

	// Start listening for collection
	go topologyController.watchTopologyChanges(telemetryChan)

	r.PathPrefix("/").Handler(http.FileServer(http.Dir(BasePath + "/public")))

}
