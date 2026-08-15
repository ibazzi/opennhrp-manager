package service

import (
	"archive/zip"
	"bytes"
	"fmt"
)

type SpokeConfigTemplateParams struct {
	InterfaceName string   `json:"interface_name"` // e.g. "gre-ha"
	LocalIP       string   `json:"local_ip"`       // Protocol IP e.g. "10.20.0.101/24"
	HubProtocolIP string   `json:"hub_protocol_ip"`// e.g. "10.20.0.1"
	HubEndpoints  []string `json:"hub_endpoints"`
	GREKey        string   `json:"gre_key,omitempty"` // e.g. "1021"
	Platform      string   `json:"platform"`       // linux, debian, openwrt
}

type ProvisioningService struct {
	nodeMgr *NodeManager
}

func NewProvisioningService(nodeMgr *NodeManager) *ProvisioningService {
	return &ProvisioningService{nodeMgr: nodeMgr}
}

func (p *ProvisioningService) GenerateOpenNHRPConf(params SpokeConfigTemplateParams) string {
	ifname := params.InterfaceName
	if ifname == "" {
		ifname = "gre-ha"
	}
	hubProto := params.HubProtocolIP
	if hubProto == "" {
		hubProto = "10.20.0.1"
	}

	var buf bytes.Buffer
	buf.WriteString("# Generated OpenNHRP Configuration for Spoke\n\n")
	buf.WriteString(fmt.Sprintf("interface %s\n", ifname))
	buf.WriteString("  holding-time 300\n")
	buf.WriteString("  shortcut\n")
	buf.WriteString("  redirect\n")
	buf.WriteString("  non-caching\n\n")

	for _, endpoint := range params.HubEndpoints {
		if endpoint != "" {
			buf.WriteString(fmt.Sprintf("  map %s %s register\n", hubProto, endpoint))
		}
	}

	return buf.String()
}

func (p *ProvisioningService) GenerateSetupScript(params SpokeConfigTemplateParams) string {
	ifname := params.InterfaceName
	if ifname == "" {
		ifname = "gre-ha"
	}
	localIP := params.LocalIP
	if localIP == "" {
		localIP = "10.20.0.100/24"
	}

	var buf bytes.Buffer
	buf.WriteString("#!/bin/bash\n")
	buf.WriteString("set -e\n\n")
	buf.WriteString(fmt.Sprintf("# Setup mGRE interface %s\n", ifname))
	buf.WriteString(fmt.Sprintf("ip link del %s 2>/dev/null || true\n", ifname))

	if params.GREKey != "" {
		buf.WriteString(fmt.Sprintf("ip tunnel add %s mode gre key %s ttl 64\n", ifname, params.GREKey))
	} else {
		buf.WriteString(fmt.Sprintf("ip tunnel add %s mode gre ttl 64\n", ifname))
	}

	buf.WriteString(fmt.Sprintf("ip addr add %s dev %s\n", localIP, ifname))
	buf.WriteString(fmt.Sprintf("ip link set %s up\n\n", ifname))
	buf.WriteString("# Restart opennhrp daemon\n")
	buf.WriteString("systemctl restart opennhrp || /etc/init.d/opennhrp restart\n")
	buf.WriteString("echo 'Spoke mGRE and OpenNHRP started successfully.'\n")

	return buf.String()
}

func (p *ProvisioningService) BuildPackageZip(params SpokeConfigTemplateParams, keyringBytes []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	confContent := p.GenerateOpenNHRPConf(params)
	confFile, err := zipWriter.Create("opennhrp.conf")
	if err != nil {
		return nil, err
	}
	_, _ = confFile.Write([]byte(confContent))

	scriptContent := p.GenerateSetupScript(params)
	scriptFile, err := zipWriter.Create("setup-gre.sh")
	if err != nil {
		return nil, err
	}
	_, _ = scriptFile.Write([]byte(scriptContent))

	if len(keyringBytes) > 0 {
		keyFile, err := zipWriter.Create(fmt.Sprintf("ha/%s.keys", params.InterfaceName))
		if err == nil {
			_, _ = keyFile.Write(keyringBytes)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
