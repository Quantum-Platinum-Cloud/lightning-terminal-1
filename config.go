package terminal

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/btcsuite/btcutil"
	"github.com/jessevdk/go-flags"
	"github.com/lightninglabs/faraday"
	"github.com/lightninglabs/faraday/chain"
	"github.com/lightninglabs/faraday/frdrpc"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/loopd"
	"github.com/lightninglabs/pool"
	"github.com/lightningnetwork/lnd"
	"github.com/lightningnetwork/lnd/build"
	"github.com/lightningnetwork/lnd/cert"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/mwitkow/go-conntrack/connhelpers"
	"golang.org/x/crypto/acme/autocert"
)

const (
	defaultHTTPSListen = "127.0.0.1:8443"

	uiPasswordMinLength = 8

	ModeIntegrated = "integrated"
	ModeRemote     = "remote"

	defaultLndMode = ModeRemote

	defaultConfigFilename = "lit.conf"

	defaultLogLevel       = "info"
	defaultMaxLogFiles    = 3
	defaultMaxLogFileSize = 10

	defaultLetsEncryptSubDir          = "letsencrypt"
	defaultLetsEncryptListen          = ":80"
	defaultSelfSignedCertOrganization = "litd autogenerated cert"

	defaultLogDirname  = "logs"
	defaultLogFilename = "litd.log"

	defaultTLSCertFilename = "tls.cert"
	defaultTLSKeyFilename  = "tls.key"

	defaultNetwork            = "mainnet"
	defaultRemoteLndRpcServer = "localhost:10009"
	defaultLndChainSubDir     = "chain"
	defaultLndChain           = "bitcoin"
	defaultLndMacaroon        = "admin.macaroon"
)

var (
	lndDefaultConfig     = lnd.DefaultConfig()
	faradayDefaultConfig = faraday.DefaultConfig()
	loopDefaultConfig    = loopd.DefaultConfig()
	poolDefaultConfig    = pool.DefaultConfig()

	// defaultLitDir is the default directory where LiT tries to find its
	// configuration file and store its data (in remote lnd node). This is a
	// directory in the user's application data, for example:
	//   C:\Users\<username>\AppData\Local\Lit on Windows
	//   ~/.lit on Linux
	//   ~/Library/Application Support/Lit on MacOS
	defaultLitDir = btcutil.AppDataDir("lit", false)

	// defaultTLSCertPath is the default full path of the autogenerated TLS
	// certificate that is created in remote lnd mode.
	defaultTLSCertPath = filepath.Join(
		defaultLitDir, defaultTLSCertFilename,
	)

	// defaultTLSKeyPath is the default full path of the autogenerated TLS
	// key that is created in remote lnd mode.
	defaultTLSKeyPath = filepath.Join(defaultLitDir, defaultTLSKeyFilename)

	// defaultConfigFile is the default path for the LiT configuration file
	// that is always attempted to be loaded.
	defaultConfigFile = filepath.Join(defaultLitDir, defaultConfigFilename)

	// defaultLogDir is the default directory in which LiT writes its log
	// files in remote lnd mode.
	defaultLogDir = filepath.Join(defaultLitDir, defaultLogDirname)

	// defaultLetsEncryptDir is the default directory in which LiT writes
	// its Let's Encrypt files.
	defaultLetsEncryptDir = filepath.Join(
		defaultLitDir, defaultLetsEncryptSubDir,
	)

	// defaultRemoteLndMacDir is the default directory we assume for a local
	// lnd node to store all its macaroon files.
	defaultRemoteLndMacDir = filepath.Join(
		lndDefaultConfig.DataDir, defaultLndChainSubDir,
		defaultLndChain, defaultNetwork,
	)
)

// Config is the main configuration struct of lightning-terminal. It contains
// all config items of its enveloping subservers, each prefixed with their
// daemon's short name.
type Config struct {
	HTTPSListen    string `long:"httpslisten" description:"The host:port to listen for incoming HTTP/2 connections on for the web UI only."`
	HTTPListen     string `long:"insecure-httplisten" description:"The host:port to listen on with TLS disabled. This is dangerous to enable as credentials will be submitted without encryption. Should only be used in combination with Tor hidden services or other external encryption."`
	UIPassword     string `long:"uipassword" description:"The password that must be entered when using the loop UI. use a strong password to protect your node from unauthorized access through the web UI."`
	UIPasswordFile string `long:"uipassword_file" description:"Same as uipassword but instead of passing in the value directly, read the password from the specified file."`
	UIPasswordEnv  string `long:"uipassword_env" description:"Same as uipassword but instead of passing in the value directly, read the password from the specified environment variable."`

	LetsEncrypt       bool   `long:"letsencrypt" description:"Use Let's Encrypt to create a TLS certificate for the UI instead of using lnd's TLS certificate. Port 80 must be free to listen on and must be reachable from the internet for this to work."`
	LetsEncryptHost   string `long:"letsencrypthost" description:"The host name to create a Let's Encrypt certificate for."`
	LetsEncryptDir    string `long:"letsencryptdir" description:"The directory where the Let's Encrypt library will store its key and certificate."`
	LetsEncryptListen string `long:"letsencryptlisten" description:"The IP:port on which LiT will listen for Let's Encrypt challenges. Let's Encrypt will always try to contact on port 80. Often non-root processes are not allowed to bind to ports lower than 1024. This configuration option allows a different port to be used, but must be used in combination with port forwarding from port 80. This configuration can also be used to specify another IP address to listen on, for example an IPv6 address."`

	LndMode string `long:"lnd-mode" description:"The mode to run lnd in, either 'integrated' (default) or 'remote'. 'integrated' means lnd is started alongside the UI and everything is stored in lnd's main data directory, configure everything by using the --lnd.* flags. 'remote' means the UI connects to an existing lnd node and acts as a proxy for gRPC calls to it. In the remote node LiT creates its own directory for log and configuration files, configure everything using the --remote.* flags." choice:"integrated" choice:"remote"`

	LitDir     string `long:"lit-dir" description:"The main directory where LiT looks for its configuration file. If LiT is running in 'remote' lnd mode, this is also the directory where the TLS certificates and log files are stored by default."`
	ConfigFile string `long:"configfile" description:"Path to LiT's configuration file."`

	Remote *RemoteConfig `group:"Remote mode options (use when lnd-mode=remote)" namespace:"remote"`

	Faraday *faraday.Config `group:"Faraday options" namespace:"faraday"`
	Loop    *loopd.Config   `group:"Loop options" namespace:"loop"`
	Pool    *pool.Config    `group:"pool" namespace:"pool"`

	Lnd *lnd.Config `group:"Integrated lnd (use when lnd-mode=integrated)" namespace:"lnd"`

	// faradayRpcConfig is a subset of faraday's full configuration that is
	// passed into faraday's RPC server.
	faradayRpcConfig *frdrpc.Config

	// network is the Bitcoin network we're running on. This will be parsed
	// and set when the configuration is loaded, either from
	// `lnd.bitcoin.mainnet|testnet|regtest` or from `remote.lnd.network`.
	network string
}

// RemoteConfig holds the configuration parameters that are needed when running
// LiT in the "remote" lnd mode.
type RemoteConfig struct {
	LitTLSCertPath string `long:"lit-tlscertpath" description:"For lnd remote mode only: Path to write the self signed TLS certificate for LiT's RPC and REST proxy service (if Let's Encrypt is not used)."`
	LitTLSKeyPath  string `long:"lit-tlskeypath" description:"For lnd remote mode only: Path to write the self signed TLS private key for LiT's RPC and REST proxy service (if Let's Encrypt is not used)."`

	LitLogDir         string `long:"lit-logdir" description:"For lnd remote mode only: Directory to log output."`
	LitMaxLogFiles    int    `long:"lit-maxlogfiles" description:"For lnd remote mode only: Maximum logfiles to keep (0 for no rotation)"`
	LitMaxLogFileSize int    `long:"lit-maxlogfilesize" description:"For lnd remote mode only: Maximum logfile size in MB"`

	LitDebugLevel string `long:"lit-debuglevel" description:"For lnd remote mode only: Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems."`

	Lnd *RemoteDaemonConfig `group:"Remote lnd (use when lnd-mode=remote)" namespace:"lnd"`
}

// RemoteDaemonConfig holds the configuration parameters that are needed to
// connect to a remote daemon like lnd for example.
type RemoteDaemonConfig struct {
	Network string `long:"network" description:"The network the remote daemon runs on" choice:"regtest" choice:"testnet" choice:"mainnet" choice:"simnet"`

	// RPCServer is host:port that the remote daemon's RPC server is
	// listening on.
	RPCServer string `long:"rpcserver" description:"The host:port that the remote daemon is listening for RPC connections on."`

	// MacaroonDir is the directory that contains all the macaroon files
	// required for the remote connection.
	MacaroonDir string `long:"macaroondir" description:"DEPRECATED: Use macaroonpath. The directory containing all lnd macaroons to use for the remote connection."`

	// MacaroonPath is the path to the single macaroon that should be used
	// instead of needing to specify the macaroon directory that contains
	// all of lnd's macaroons. The specified macaroon MUST have all
	// permissions that all the subservers use, otherwise permission errors
	// will occur.
	MacaroonPath string `long:"macaroonpath" description:"The full path to the single macaroon to use, either the admin.macaroon or a custom baked one. Cannot be specified at the same time as macaroondir. A custom macaroon must contain ALL permissions required for all subservers to work, otherwise permission errors will occur."`

	// TLSCertPath is the path to the tls cert of the remote daemon that
	// should be used to verify the TLS identity of the remote RPC server.
	TLSCertPath string `long:"tlscertpath" description:"The full path to the remote daemon's TLS cert to use for RPC connection verification."`
}

// lndConnectParams returns the connection parameters to connect to the local
// lnd instance.
func (c *Config) lndConnectParams() (string, lndclient.Network, string,
	string) {

	// In remote lnd mode, we just pass along what was configured in the
	// remote section of the lnd config.
	if c.LndMode == ModeRemote {
		// Because we now have the option to specify a single, custom
		// macaroon to the lndclient, we either use the single macaroon
		// indicated by the user or the admin macaroon from the mac dir
		// that was specified.
		macPath := path.Join(
			lncfg.CleanAndExpandPath(c.Remote.Lnd.MacaroonDir),
			defaultLndMacaroon,
		)
		if c.Remote.Lnd.MacaroonPath != "" {
			macPath = lncfg.CleanAndExpandPath(
				c.Remote.Lnd.MacaroonPath,
			)
		}

		return c.Remote.Lnd.RPCServer,
			lndclient.Network(c.network),
			lncfg.CleanAndExpandPath(c.Remote.Lnd.TLSCertPath),
			macPath
	}

	// When we start lnd internally, we take the listen address as
	// the client dial address. But with TLS enabled by default, we
	// cannot call 0.0.0.0 internally when dialing lnd as that IP
	// address isn't in the cert. We need to rewrite it to the
	// loopback address.
	lndDialAddr := c.Lnd.RPCListeners[0].String()
	switch {
	case strings.Contains(lndDialAddr, "0.0.0.0"):
		lndDialAddr = strings.Replace(
			lndDialAddr, "0.0.0.0", "127.0.0.1", 1,
		)

	case strings.Contains(lndDialAddr, "[::]"):
		lndDialAddr = strings.Replace(
			lndDialAddr, "[::]", "[::1]", 1,
		)
	}

	return lndDialAddr, lndclient.Network(c.network),
		c.Lnd.TLSCertPath, c.Lnd.AdminMacPath
}

// defaultConfig returns a configuration struct with all default values set.
func defaultConfig() *Config {
	return &Config{
		HTTPSListen: defaultHTTPSListen,
		Remote: &RemoteConfig{
			LitTLSCertPath:    defaultTLSCertPath,
			LitTLSKeyPath:     defaultTLSKeyPath,
			LitDebugLevel:     defaultLogLevel,
			LitLogDir:         defaultLogDir,
			LitMaxLogFiles:    defaultMaxLogFiles,
			LitMaxLogFileSize: defaultMaxLogFileSize,
			Lnd: &RemoteDaemonConfig{
				Network:     defaultNetwork,
				RPCServer:   defaultRemoteLndRpcServer,
				MacaroonDir: defaultRemoteLndMacDir,
				TLSCertPath: lndDefaultConfig.TLSCertPath,
			},
		},
		LndMode:           defaultLndMode,
		Lnd:               &lndDefaultConfig,
		LitDir:            defaultLitDir,
		LetsEncryptListen: defaultLetsEncryptListen,
		LetsEncryptDir:    defaultLetsEncryptDir,
		ConfigFile:        defaultConfigFile,
		Faraday:           &faradayDefaultConfig,
		faradayRpcConfig:  &frdrpc.Config{},
		Loop:              &loopDefaultConfig,
		Pool:              &poolDefaultConfig,
	}
}

// loadAndValidateConfig loads the terminal's main configuration and validates
// its content.
func loadAndValidateConfig() (*Config, error) {
	// Start with the default configuration.
	preCfg := defaultConfig()

	// Pre-parse the command line options to pick up an alternative config
	// file.
	_, err := flags.Parse(preCfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing flags: %w", err)
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.Lnd.ShowVersion {
		fmt.Println(appName, "version", build.Version(),
			"commit="+build.Commit)
		os.Exit(0)
	}

	// Load the main configuration file and parse any command line options.
	// This function will also set up logging properly.
	cfg, err := loadConfigFile(preCfg, usageMessage)
	if err != nil {
		return nil, err
	}

	// With the validated config obtained, we now know that the root logging
	// system of lnd is initialized and we can hook up our own loggers now.
	SetupLoggers(cfg.Lnd.LogWriter)

	// Validate the lightning-terminal config options.
	litDir := lnd.CleanAndExpandPath(preCfg.LitDir)
	cfg.LetsEncryptDir = lncfg.CleanAndExpandPath(cfg.LetsEncryptDir)
	if litDir != defaultLitDir {
		if cfg.LetsEncryptDir == defaultLetsEncryptDir {
			cfg.LetsEncryptDir = filepath.Join(
				litDir, defaultLetsEncryptSubDir,
			)
		}
	}
	if cfg.LetsEncrypt {
		if cfg.LetsEncryptHost == "" {
			return nil, fmt.Errorf("host must be set when using " +
				"let's encrypt")
		}

		// Create the directory if we're going to use Let's Encrypt.
		if err := makeDirectories(cfg.LetsEncryptDir); err != nil {
			return nil, err
		}
	}
	err = readUIPassword(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not read UI password: %v", err)
	}
	if len(cfg.UIPassword) < uiPasswordMinLength {
		return nil, fmt.Errorf("please set a strong password for the "+
			"UI, at least %d characters long", uiPasswordMinLength)
	}

	// Initiate our listeners. For now, we only support listening on one
	// port at a time because we can only pass in one pre-configured RPC
	// listener into lnd.
	if len(cfg.Lnd.RPCListeners) > 1 {
		return nil, fmt.Errorf("litd only supports one RPC listener " +
			"at a time")
	}

	// Some of the subservers' configuration options won't have any effect
	// (like the log or lnd options) as they will be taken from lnd's config
	// struct. Others we want to force to be the same as lnd so the user
	// doesn't have to set them manually, like the network for example.
	cfg.Loop.Network = cfg.network
	if err := loopd.Validate(cfg.Loop); err != nil {
		return nil, err
	}

	cfg.Pool.Network = cfg.network
	if err := pool.Validate(cfg.Pool); err != nil {
		return nil, err
	}

	cfg.Faraday.Network = cfg.network
	if err := faraday.ValidateConfig(cfg.Faraday); err != nil {
		return nil, err
	}
	cfg.faradayRpcConfig.FaradayDir = cfg.Faraday.FaradayDir
	cfg.faradayRpcConfig.MacaroonPath = cfg.Faraday.MacaroonPath

	// If the client chose to connect to a bitcoin client, get one now.
	if cfg.Faraday.ChainConn {
		cfg.faradayRpcConfig.BitcoinClient, err = chain.NewBitcoinClient(
			cfg.Faraday.Bitcoin,
		)
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// loadConfigFile loads and sanitizes the lit main configuration from the config
// file or command line arguments (or both).
func loadConfigFile(preCfg *Config, usageMessage string) (*Config, error) {
	// If the config file path has not been modified by the user, then we'll
	// use the default config file path. However, if the user has modified
	// their litdir, then we should assume they intend to use the config
	// file within it.
	litDir := lnd.CleanAndExpandPath(preCfg.LitDir)
	configFilePath := lnd.CleanAndExpandPath(preCfg.ConfigFile)
	if litDir != defaultLitDir {
		if configFilePath == defaultConfigFile {
			configFilePath = filepath.Join(
				litDir, defaultConfigFilename,
			)
		}
	}

	// Next, load any additional configuration options from the file.
	var configFileError error
	cfg := preCfg
	if err := flags.IniParse(configFilePath, cfg); err != nil {
		// If it's a parsing related error, then we'll return
		// immediately, otherwise we can proceed as possibly the config
		// file doesn't exist which is OK.
		if _, ok := err.(*flags.IniError); ok {
			return nil, err
		}

		configFileError = err
	}

	// Finally, parse the remaining command line options again to ensure
	// they take precedence.
	if _, err := flags.Parse(cfg); err != nil {
		return nil, err
	}

	// Now make sure we create the LiT directory if it doesn't yet exist.
	if err := makeDirectories(litDir); err != nil {
		return nil, err
	}

	switch cfg.LndMode {
	// In case we are running lnd in-process, let's make sure its
	// configuration is fully valid. This also sets up the main logger that
	// logs to a sub-directory in the .lnd folder.
	case ModeIntegrated:
		var err error
		cfg.Lnd, err = lnd.ValidateConfig(*cfg.Lnd, usageMessage)
		if err != nil {
			return nil, err
		}
		cfg.network, err = getNetwork(cfg.Lnd.Bitcoin)
		if err != nil {
			return nil, err
		}

	// In remote lnd mode we skip the validation of the lnd configuration
	// and instead just set up the logging (that would be done by lnd if it
	// were running in the same process).
	case ModeRemote:
		if err := validateRemoteModeConfig(cfg); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("invalid lnd mode %v", cfg.LndMode)
	}

	// Warn about missing config file only after all other configuration is
	// done. This prevents the warning on help messages and invalid options.
	// Note this should go directly before the return.
	if configFileError != nil {
		log.Warnf("%v", configFileError)
	}

	return cfg, nil
}

// validateRemoteModeConfig validates the terminal's own configuration
// parameters that are only used in the "remote" lnd mode.
func validateRemoteModeConfig(cfg *Config) error {
	r := cfg.Remote

	// Validate the network as in the remote node it's provided as a string
	// instead of a series of boolean flags.
	if _, err := lndclient.Network(r.Lnd.Network).ChainParams(); err != nil {
		return fmt.Errorf("error validating lnd remote network: %v", err)
	}
	cfg.network = r.Lnd.Network

	// Users can either specify the macaroon directory or the custom
	// macaroon to use, but not both.
	if r.Lnd.MacaroonDir != defaultRemoteLndMacDir &&
		r.Lnd.MacaroonPath != "" {

		return fmt.Errorf("cannot set both macaroon dir and macaroon " +
			"path")
	}

	// If the remote lnd's network isn't the default, we also check if we
	// need to adjust the default macaroon directory so the user can only
	// specify --network=testnet for example if everything else is using
	// the defaults.
	if r.Lnd.Network != defaultNetwork &&
		r.Lnd.MacaroonDir == defaultRemoteLndMacDir {

		r.Lnd.MacaroonDir = filepath.Join(
			lndDefaultConfig.DataDir, defaultLndChainSubDir,
			defaultLndChain, r.Lnd.Network,
		)
	}

	// If the provided lit directory is not the default, we'll modify the
	// path to all of the files and directories that will live within it.
	litDir := lnd.CleanAndExpandPath(cfg.LitDir)
	if litDir != defaultLitDir {
		r.LitTLSCertPath = filepath.Join(litDir, defaultTLSCertFilename)
		r.LitTLSKeyPath = filepath.Join(litDir, defaultTLSKeyFilename)
		r.LitLogDir = filepath.Join(litDir, defaultLogDirname)
	}

	r.LitTLSCertPath = lncfg.CleanAndExpandPath(r.LitTLSCertPath)
	r.LitTLSKeyPath = lncfg.CleanAndExpandPath(r.LitTLSKeyPath)
	r.LitLogDir = lncfg.CleanAndExpandPath(r.LitLogDir)

	// Make sure the parent directories of our certificate files exist. We
	// don't need to do the same for the log dir as the log rotator will do
	// just that.
	if err := makeDirectories(filepath.Dir(r.LitTLSCertPath)); err != nil {
		return err
	}
	if err := makeDirectories(filepath.Dir(r.LitTLSKeyPath)); err != nil {
		return err
	}

	// In remote mode, we don't call lnd's ValidateConfig that sets up a
	// logging backend for us. We need to manually create and start one.
	logWriter := build.NewRotatingLogWriter()
	cfg.Lnd.LogWriter = logWriter
	err := logWriter.InitLogRotator(
		filepath.Join(r.LitLogDir, cfg.network, defaultLogFilename),
		r.LitMaxLogFileSize, r.LitMaxLogFiles,
	)
	if err != nil {
		return fmt.Errorf("log rotation setup failed: %v", err.Error())
	}

	// Parse, validate, and set debug log level(s).
	return build.ParseAndSetDebugLevels(
		cfg.Remote.LitDebugLevel, logWriter,
	)
}

func getNetwork(cfg *lncfg.Chain) (string, error) {
	switch {
	case cfg.MainNet:
		return "mainnet", nil

	case cfg.TestNet3:
		return "testnet", nil

	case cfg.RegTest:
		return "regtest", nil

	case cfg.SimNet:
		return "simnet", nil

	default:
		return "", fmt.Errorf("no network selected")
	}
}

// readUIPassword reads the password for the UI either from the command line
// flag, a file specified or an environment variable.
func readUIPassword(config *Config) error {
	// A password is passed in as a command line flag (or config file
	// variable) directly.
	if len(strings.TrimSpace(config.UIPassword)) > 0 {
		config.UIPassword = strings.TrimSpace(config.UIPassword)
		return nil
	}

	// A file that contains the password is specified.
	if len(strings.TrimSpace(config.UIPasswordFile)) > 0 {
		content, err := ioutil.ReadFile(strings.TrimSpace(
			config.UIPasswordFile,
		))
		if err != nil {
			return fmt.Errorf("could not read file %s: %v",
				config.UIPasswordFile, err)
		}
		config.UIPassword = strings.TrimSpace(string(content))
		return nil
	}

	// The name of an environment variable was specified.
	if len(strings.TrimSpace(config.UIPasswordEnv)) > 0 {
		content := os.Getenv(strings.TrimSpace(config.UIPasswordEnv))
		if len(content) == 0 {
			return fmt.Errorf("environment variable %s is empty",
				config.UIPasswordEnv)
		}
		config.UIPassword = strings.TrimSpace(content)
		return nil
	}

	return fmt.Errorf("mandatory password for UI not configured. specify " +
		"either a password directly or a file or environment " +
		"variable that contains the password")
}

func buildTLSConfigForHttp2(config *Config) (*tls.Config, error) {
	var tlsConfig *tls.Config

	switch {
	case config.LetsEncrypt:
		serverName := config.LetsEncryptHost
		if serverName == "" {
			return nil, errors.New("let's encrypt host name " +
				"option is required for using let's encrypt")
		}

		log.Infof("Setting up Let's Encrypt for server %v", serverName)

		certDir := config.LetsEncryptDir
		log.Infof("Setting up Let's Encrypt with cache dir %v", certDir)

		manager := autocert.Manager{
			Cache:      autocert.DirCache(certDir),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(serverName),
		}

		go func() {
			log.Infof("Listening for Let's Encrypt challenges on "+
				"%v", config.LetsEncryptListen)

			err := http.ListenAndServe(
				config.LetsEncryptListen,
				manager.HTTPHandler(nil),
			)
			if err != nil {
				log.Errorf("Error starting Let's Encrypt "+
					"HTTP listener on port 80: %v", err)
			}
		}()
		tlsConfig = &tls.Config{
			GetCertificate: manager.GetCertificate,
		}

	case config.LndMode == ModeRemote:
		tlsCertPath := config.Remote.LitTLSCertPath
		tlsKeyPath := config.Remote.LitTLSKeyPath

		if !lnrpc.FileExists(tlsCertPath) &&
			!lnrpc.FileExists(tlsKeyPath) {

			err := cert.GenCertPair(
				defaultSelfSignedCertOrganization, tlsCertPath,
				tlsKeyPath, nil, nil, false,
				cert.DefaultAutogenValidity,
			)
			if err != nil {
				return nil, fmt.Errorf("failed creating "+
					"self-signed cert: %v", err)
			}
		}

		tlsCert, _, err := cert.LoadCert(tlsCertPath, tlsKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed reading TLS server "+
				"keys: %v", err)
		}
		tlsConfig = cert.TLSConfFromCert(tlsCert)

	default:
		tlsCert, _, err := cert.LoadCert(
			config.Lnd.TLSCertPath, config.Lnd.TLSKeyPath,
		)
		if err != nil {
			return nil, fmt.Errorf("failed reading TLS server "+
				"keys: %v", err)
		}
		tlsConfig = cert.TLSConfFromCert(tlsCert)
	}

	// lnd's cipher suites are too restrictive for HTTP/2, we need to add
	// one of the default suites back to stop the HTTP/2 lib from
	// complaining.
	tlsConfig.CipherSuites = append(
		tlsConfig.CipherSuites,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	)
	tlsConfig, err := connhelpers.TlsConfigWithHttp2Enabled(tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("can't configure h2 handling: %v", err)
	}
	return tlsConfig, nil
}

// makeDirectories creates the directory given and if necessary any parent
// directories as well.
func makeDirectories(fullDir string) error {
	err := os.MkdirAll(fullDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		err := fmt.Errorf("failed to create directory %v: %v", fullDir,
			err)
		_, _ = fmt.Fprintln(os.Stderr, err)
		return err
	}

	return nil
}

// onDemandListener is a net.Listener that only actually starts to listen on a
// network port once the Accept method is called.
type onDemandListener struct {
	addr net.Addr
	lis  net.Listener
}

// Accept waits for and returns the next connection to the listener.
func (l *onDemandListener) Accept() (net.Conn, error) {
	if l.lis == nil {
		var err error
		l.lis, err = net.Listen(parseNetwork(l.addr), l.addr.String())
		if err != nil {
			return nil, err
		}
	}
	return l.lis.Accept()
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *onDemandListener) Close() error {
	return l.lis.Close()
}

// Addr returns the listener's network address.
func (l *onDemandListener) Addr() net.Addr {
	return l.addr
}

// parseNetwork parses the network type of the given address.
func parseNetwork(addr net.Addr) string {
	switch addr := addr.(type) {
	// TCP addresses resolved through net.ResolveTCPAddr give a default
	// network of "tcp", so we'll map back the correct network for the given
	// address. This ensures that we can listen on the correct interface
	// (IPv4 vs IPv6).
	case *net.TCPAddr:
		if addr.IP.To4() != nil {
			return "tcp4"
		}
		return "tcp6"

	default:
		return addr.Network()
	}
}
