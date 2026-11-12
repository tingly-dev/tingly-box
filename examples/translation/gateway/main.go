// A loopback-only development gateway using the production translation routes.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocolserver"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/translation"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func main() {
	base := flag.String("upstream", "http://127.0.0.1:18081/v1", "native OpenAI-compatible MT base URL")
	backend := flag.String("backend", translation.HunyuanMT, "hunyuan_mt or qwen_mt")
	model := flag.String("model", "local-mt", "upstream deployment alias")
	proxy := flag.String("proxy", "", "explicit provider proxy URL, if required")
	flag.Parse()
	if *backend != translation.HunyuanMT && *backend != translation.QwenMT {
		log.Fatal("unsupported translation backend")
	}
	dir, err := os.MkdirTemp("", "tingly-translation-demo-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cfg, err := config.NewConfig(config.WithConfigDir(dir), config.WithDisableMigration(), config.WithDisableBuiltIn())
	if err != nil {
		log.Fatal(err)
	}
	cfg.ModelToken = "local-translation-demo"
	if err := cfg.AddProvider(&typ.Provider{UUID: "demo-mt", Name: "demo-mt", Enabled: true, AuthType: typ.AuthTypeAPIKey,
		Token: os.Getenv("TINGLY_TRANSLATION_API_KEY"), NoKeyRequired: os.Getenv("TINGLY_TRANSLATION_API_KEY") == "",
		APIStyle: protocol.APIStyleOpenAI, APIBase: *base, ProxyURL: *proxy, Timeout: 300}); err != nil {
		log.Fatal(err)
	}
	if err := cfg.AddRule(typ.Rule{UUID: "demo-translation", Scenario: typ.ScenarioCustom, RequestModel: "translate", Active: true,
		Services: []*loadbalance.Service{{Provider: "demo-mt", Model: *model, TranslationBackend: *backend, Active: true}}}); err != nil {
		log.Fatal(err)
	}
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := protocolserver.NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	h := protocolserver.NewHandler(protocolserver.ProtocolHandlerDeps{Config: cfg, ClientPool: client.NewClientPool(), LoadBalancer: lb, HealthMonitor: hm,
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, protocolserver.NewAffinityStore(0), lb))})
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h.RegisterRoutes(r, middleware.NewAuthMiddleware(cfg, nil, nil, nil).ModelAuthMiddleware())
	log.Print("Development gateway: http://127.0.0.1:18080/tingly/custom/v1/translate")
	if err := r.Run("127.0.0.1:18080"); err != nil {
		log.Fatal(err)
	}
}
