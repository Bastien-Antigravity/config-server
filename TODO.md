check this part : 
func main() {
	// 1. Initialize Toolbox Config (which handles name/IP resolution)
	// Passing nil for specificFlags lets LoadConfig use the default flag parsing.
	appConfig, err := utilconf.LoadConfigWithLogger("standalone", nil, nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger (bootstrap)
	_, appLogger := bootstrap.Init("config-server", "standalone", "no_lock", utils.GetLogLevel("INFO"), false)
	defer appLogger.Close()

here the loading of :
_, appLogger := bootstrap.Init("config-server", "standalone", "no_lock", utils.GetLogLevel("INFO"), false)

is not relecant with 

utilconf.LoadConfigWithLogger

bootstrap.Init signature of boostrap.init should have change and the logger level should comes from appConfig



appLogger : overload with name : 	
appLogger.Info("{name} :Configuration loaded via Toolbox (network-aware)")

appLogger.Info("Starting Config Server on port %s...", *port)

normally we have implemented something to add automacally the name of the logger and then if :
	appLogger.Info("Starting Config Server on port %s...", *port)

should return in log and in console : "logger_name : Starting Config Server on port 1234..."

could you show me ? (normally it should be in flexible-logger, universal-logger or microservice-toolbox)



 Create Implementation Plan
 Get User Approval
 Refactor universal-logger/src/bootstrap/unilog.go
 Update Init and InitWithOptions logic.
 Add cloud_native profile mapping.
 Update universal-logger/src/cgo_bridge/initialize.go
 Update UniLog_Init to pass nil for cfg.
 Update universal-logger/README.md
 Fix the Quick Start documentation.
 Update config-server/cmd/config-server/main.go
 Update bootstrap.Init call to pass Toolbox's appConfig.Config.
 Update config-server/cmd/test/main.go
 Update bootstrap.Init / NewUniLog initialization logic to match changes.
 Verify everything compiles (Test go build ./... in both universal-logger and config-server).