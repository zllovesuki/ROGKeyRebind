package server

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"sync"

	"github.com/zllovesuki/G14Manager/rpc/annoucement"
	"github.com/zllovesuki/G14Manager/rpc/protocol"
	"github.com/zllovesuki/G14Manager/system/shared"
	"github.com/zllovesuki/G14Manager/system/keyboard"
	"github.com/zllovesuki/G14Manager/system/persist"
	"github.com/zllovesuki/G14Manager/system/thermal"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	featuresPersistName = "Features"
)

type ConfigListServer struct {
	protocol.UnimplementedConfigListServer

	mu        sync.RWMutex
	updatable []annoucement.Updatable
	features  shared.Features
	profiles  []thermal.Profile
}

var _ protocol.ConfigListServer = &ConfigListServer{}

func RegisterConfigListServer(s *grpc.Server, u []annoucement.Updatable) *ConfigListServer {
	server := &ConfigListServer{
		updatable: u,
		// sensible defaults
		features: shared.Features{
			FnRemap: map[uint32]uint16{
				keyboard.KeyFnLeft:  keyboard.KeyPgUp,
				keyboard.KeyFnRight: keyboard.KeyPgDown,
			},
			AutoThermal: shared.AutoThermal{
				Enabled: false,
			},
			RogRemap: []string{"Taskmgr.exe"},
		},
		profiles: thermal.GetDefaultThermalProfiles(),
	}
	protocol.RegisterConfigListServer(s, server)
	return server
}

func (f *ConfigListServer) GetCurrentList(ctx context.Context, req *emptypb.Empty) (*protocol.SetConfigsResponse, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	fnRemap := make(map[uint32]uint32)
	for k, v := range f.features.FnRemap {
		fnRemap[k] = uint32(v)
	}
	profiles := make([]*protocol.Profile, 0, 3)
	for _, p := range f.profiles {
		var val protocol.Profile_ThrottleValue
		switch p.ThrottlePlan {
		case thermal.ThrottlePlanPerformance:
			val = protocol.Profile_PERFORMANCE
		case thermal.ThrottlePlanSilent:
			val = protocol.Profile_SILENT
		case thermal.ThrottlePlanTurbo:
			val = protocol.Profile_TURBO
		}
		profiles = append(profiles, &protocol.Profile{
			Name:             p.Name,
			WindowsPowerPlan: p.WindowsPowerPlan,
			ThrottlePlan:     val,
			CPUFanCurve:      p.CPUFanCurve.String(),
			GPUFanCurve:      p.GPUFanCurve.String(),
		})
	}
	return &protocol.SetConfigsResponse{
		Success: true,
		Configs: &protocol.Configs{
			Features: &protocol.Features{
				AutoThermal: &protocol.AutoThermal{
					Enabled:          f.features.AutoThermal.Enabled,
					PluggedInProfile: f.features.AutoThermal.PluggedIn,
					UnpluggedProfile: f.features.AutoThermal.Unplugged,
				},
				FnRemap:  fnRemap,
				RogRemap: f.features.RogRemap,
			},
			Profiles: profiles,
		},
	}, nil
}

func (f *ConfigListServer) Set(ctx context.Context, req *protocol.SetConfigsRequest) (*protocol.SetConfigsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request is invalid")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	configs := req.GetConfigs()
	if configs == nil {
		return nil, fmt.Errorf("nil configs is invalid")
	}

	feats := configs.GetFeatures()
	profiles := configs.GetProfiles()

	if feats == nil && profiles == nil {
		return nil, fmt.Errorf("either features or profiles must be specified")
	}

	var newFeatures *shared.Features
	var newProfiles []thermal.Profile

	if feats != nil {
		fnRemap := make(map[uint32]uint16)
		for k, v := range feats.FnRemap {
			fnRemap[k] = uint16(v)
		}
		newFeatures = &shared.Features{
			AutoThermal: shared.AutoThermal{
				Enabled:   feats.AutoThermal.Enabled,
				PluggedIn: feats.AutoThermal.PluggedInProfile,
				Unplugged: feats.AutoThermal.UnpluggedProfile,
			},
			FnRemap:  fnRemap,
			RogRemap: feats.GetRogRemap(),
		}
	}

	if profiles != nil {
		// parse the fan table
	}

	if newFeatures != nil {
		f.features = *newFeatures
	}
	if len(newProfiles) > 0 {
		f.profiles = newProfiles
	}

	f.annouceConfigs()

	return &protocol.SetConfigsResponse{
		Success: true,
		Configs: req.GetConfigs(),
	}, nil
}

func (f *ConfigListServer) annouceConfigs() {
	featsUpdate := annoucement.Update{
		Type:   annoucement.FeaturesUpdate,
		Config: f.features,
	}
	profilesUpdate := annoucement.Update{
		Type:   annoucement.ProfilesUpdate,
		Config: f.profiles,
	}
	for _, updatable := range f.updatable {
		log.Printf("[grpc] notifying \"%s\" about features update", updatable.Name())
		go updatable.ConfigUpdate(featsUpdate)
		log.Printf("[grpc] notifying \"%s\" about profiles update", updatable.Name())
		go updatable.ConfigUpdate(profilesUpdate)
	}
}

func (f *ConfigListServer) HotReload(u []annoucement.Updatable) {
	f.mu.Lock()
	defer f.mu.Unlock()

	log.Println("[grpc] hot reloading features server")

	f.updatable = u
	f.annouceConfigs()
}

var _ persist.Registry = &ConfigListServer{}

func (f *ConfigListServer) Name() string {
	return featuresPersistName
}

func (f *ConfigListServer) Value() []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(f.features); err != nil {
		return nil
	}

	return buf.Bytes()
}

func (f *ConfigListServer) Load(v []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(v) == 0 {
		return nil
	}

	var feat shared.Features
	buf := bytes.NewBuffer(v)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&feat); err != nil {
		return err
	}

	f.features = feat

	return nil
}

func (f *ConfigListServer) Apply() error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.annouceConfigs()

	return nil
}

func (f *ConfigListServer) Close() error {
	return nil
}