package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	C "github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/utils"
	"github.com/sagernet/sing-box/log"
	"gopkg.in/yaml.v2"
)

type ProxySchema struct {
	Proxies []map[string]any `yaml:"proxies"`
}

type ProxySetProvider struct {
	*proxySetProvider
}

type proxySetProvider struct {
	*Fetcher[[]C.Outbound]
	proxies []C.Outbound

	// todo: need reimpl Helthcheck
	// healthCheck      *HealthCheck
	version          uint32
	subscriptionInfo *SubscriptionInfo
}

func (pp *proxySetProvider) Version() uint32 {
	return pp.version
}

func (pp *proxySetProvider) Name() string {
	return pp.Fetcher.Name()
}

func (pp *proxySetProvider) Update() error {
	elm, same, err := pp.Fetcher.Update()
	if err == nil && !same {
		pp.OnUpdate(elm)
	}
	return err
}

func (pp *proxySetProvider) setProxies(proxies []C.Outbound) {
	pp.proxies = proxies
	// pp.healthCheck.setProxy(proxies)
	// if pp.healthCheck.auto() {
	// 	go pp.healthCheck.check()
	// }
}

func (pp *proxySetProvider) Initial() error {
	elm, err := pp.Fetcher.Initial()
	if err != nil {
		return err
	}
	pp.OnUpdate(elm)
	pp.getSubscriptionInfo()
	pp.closeAllConnections()
	return nil
}

func (pp *proxySetProvider) Proxies() []C.Outbound {
	return pp.proxies
}

func (pp *proxySetProvider) getSubscriptionInfo() {
	if pp.VehicleType() != HTTP {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*90)
		defer cancel()
		resp, err := HttpRequest(ctx, pp.Vehicle().(*HTTPVehicle).Url(),
			http.MethodGet, http.Header{"User-Agent": {"clash"}}, nil)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		userInfoStr := strings.TrimSpace(resp.Header.Get("subscription-userinfo"))
		if userInfoStr == "" {
			resp2, err := HttpRequest(ctx, pp.Vehicle().(*HTTPVehicle).Url(),
				http.MethodGet, http.Header{"User-Agent": {"Quantumultx"}}, nil)
			if err != nil {
				return
			}
			defer resp2.Body.Close()
			userInfoStr = strings.TrimSpace(resp2.Header.Get("subscription-userinfo"))
			if userInfoStr == "" {
				return
			}
		}
		pp.subscriptionInfo, err = NewSubscriptionInfo(userInfoStr)
		if err != nil {
			log.Warn("[Provider] get subscription-userinfo: %e", err)
		}
	}()
}

func (pp *proxySetProvider) closeAllConnections() {
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		for _, chain := range c.Chains() {
			if chain == pp.Name() {
				_ = c.Close()
				break
			}
		}
		return true
	})
}

func stopProxyProvider(pd *ProxySetProvider) {
	//	pd.healthCheck.close()
	_ = pd.Fetcher.Destroy()
}

func NewProxySetProvider(name string, interval time.Duration, filter string, excludeFilter string, excludeType string, dialerProxy string, vehicle Vehicle, hc any) (*ProxySetProvider, error) {
	excludeFilterReg, err := regexp2.Compile(excludeFilter, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid excludeFilter regex: %w", err)
	}
	var excludeTypeArray []string
	if excludeType != "" {
		excludeTypeArray = strings.Split(excludeType, "|")
	}

	var filterRegs []*regexp2.Regexp
	for _, filter := range strings.Split(filter, "`") {
		filterReg, err := regexp2.Compile(filter, 0)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
		filterRegs = append(filterRegs, filterReg)
	}

	// if hc.auto() {
	// 	go hc.process()
	// }

	pd := &proxySetProvider{
		proxies: []C.Outbound{},
		//healthCheck: hc,
	}

	fetcher := NewFetcher[[]C.Outbound](name, interval, vehicle, proxiesParseAndFilter(filter, excludeFilter, excludeTypeArray, filterRegs, excludeFilterReg, dialerProxy), proxiesOnUpdate(pd))
	pd.Fetcher = fetcher
	wrapper := &ProxySetProvider{pd}
	runtime.SetFinalizer(wrapper, stopProxyProvider)
	return wrapper, nil
}

// CompatibleProvider for auto gc
type CompatibleProvider struct {
	*compatibleProvider
}

type compatibleProvider struct {
	name string
	//	healthCheck *HealthCheck
	proxies []C.Outbound
	version uint32
}

func (cp *compatibleProvider) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name":        cp.Name(),
		"type":        cp.Type().String(),
		"vehicleType": cp.VehicleType().String(),
		"proxies":     cp.Proxies(),
		//	"testUrl":     cp.healthCheck.url,
	})
}

func (cp *compatibleProvider) Version() uint32 {
	return cp.version
}

func (cp *compatibleProvider) Name() string {
	return cp.name
}

func (cp *compatibleProvider) HealthCheck() {
	// cp.healthCheck.check()
}

func (cp *compatibleProvider) Update() error {
	return nil
}

func (cp *compatibleProvider) Initial() error {
	return nil
}

func (cp *compatibleProvider) VehicleType() VehicleType {
	return Compatible
}

func (cp *compatibleProvider) Type() ProviderType {
	return Proxy
}

func (cp *compatibleProvider) Proxies() []C.Outbound {
	return cp.proxies
}

func (cp *compatibleProvider) Touch() {
	// cp.healthCheck.touch()
}

func (cp *compatibleProvider) RegisterHealthCheckTask(url string, expectedStatus utils.IntRanges[uint16], filter string, interval uint) {
	// cp.healthCheck.registerHealthCheckTask(url, expectedStatus, filter, interval)
}

func stopCompatibleProvider(pd *CompatibleProvider) {
	// pd.healthCheck.close()
}

func NewCompatibleProvider(name string, proxies []C.Outbound, hc any) (*CompatibleProvider, error) {
	if len(proxies) == 0 {
		return nil, errors.New("provider need one proxy at least")
	}

	// if hc.auto() {
	// 	go hc.process()
	// }

	pd := &compatibleProvider{
		name:    name,
		proxies: proxies,
		//		healthCheck: hc,
	}

	wrapper := &CompatibleProvider{pd}
	runtime.SetFinalizer(wrapper, stopCompatibleProvider)
	return wrapper, nil
}

func proxiesOnUpdate(pd *proxySetProvider) func([]C.Outbound) {
	return func(elm []C.Outbound) {
		pd.setProxies(elm)
		pd.version += 1
		pd.getSubscriptionInfo()
	}
}

func proxiesParseAndFilter(filter string, excludeFilter string, excludeTypeArray []string, filterRegs []*regexp2.Regexp, excludeFilterReg *regexp2.Regexp, dialerProxy string) Parser[[]C.Outbound] {
	return func(buf []byte) ([]C.Outbound, error) {
		schema := &ProxySchema{}

		if err := yaml.Unmarshal(buf, schema); err != nil {
			//TODO: need support v2ray subscript base64 format
			// proxies, err1 := convert.ConvertsV2Ray(buf)
			// if err1 != nil {
			// 	return nil, fmt.Errorf("%w, %w", err, err1)
			// }
			// schema.Proxies = proxies
		}

		if schema.Proxies == nil {
			return nil, errors.New("file must have a `proxies` field")
		}

		proxies := []C.Outbound{}
		proxiesSet := map[string]struct{}{}
		for _, filterReg := range filterRegs {
			for idx, mapping := range schema.Proxies {
				if nil != excludeTypeArray && len(excludeTypeArray) > 0 {
					mType, ok := mapping["type"]
					if !ok {
						continue
					}
					pType, ok := mType.(string)
					if !ok {
						continue
					}
					flag := false
					for i := range excludeTypeArray {
						if strings.EqualFold(pType, excludeTypeArray[i]) {
							flag = true
							break
						}

					}
					if flag {
						continue
					}

				}
				mName, ok := mapping["name"]
				if !ok {
					continue
				}
				name, ok := mName.(string)
				if !ok {
					continue
				}
				if len(excludeFilter) > 0 {
					if mat, _ := excludeFilterReg.FindStringMatch(name); mat != nil {
						continue
					}
				}
				if len(filter) > 0 {
					if mat, _ := filterReg.FindStringMatch(name); mat == nil {
						continue
					}
				}
				if _, ok := proxiesSet[name]; ok {
					continue
				}
				if len(dialerProxy) > 0 {
					mapping["dialer-proxy"] = dialerProxy
				}
				proxy, err := parseProxy(mapping)
				if err != nil {
					return nil, fmt.Errorf("proxy %d error: %w", idx, err)
				}
				proxiesSet[name] = struct{}{}
				proxies = append(proxies, proxy)
			}
		}

		if len(proxies) == 0 {
			if len(filter) > 0 {
				return nil, errors.New("doesn't match any proxy, please check your filter")
			}
			return nil, errors.New("file doesn't have any proxy")
		}

		return proxies, nil
	}
}
