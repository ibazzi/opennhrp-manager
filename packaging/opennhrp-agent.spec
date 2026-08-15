Name: opennhrp-agent
Version: %{version}
Release: 1%{?dist}
Summary: OpenNHRP management agent
License: MIT
Requires: opennhrp, ca-certificates
Source0: opennhrp-agent
Source1: opennhrp-agent.service
Source2: agent.env

%description
Secure outbound Agent connection for centrally managed OpenNHRP Hub and Spoke nodes.

%install
install -D -m 0755 %{SOURCE0} %{buildroot}/usr/sbin/opennhrp-agent
install -D -m 0644 %{SOURCE1} %{buildroot}/usr/lib/systemd/system/opennhrp-agent.service
install -D -m 0600 %{SOURCE2} %{buildroot}%{_sysconfdir}/opennhrp-agent/agent.env

%files
/usr/sbin/opennhrp-agent
/usr/lib/systemd/system/opennhrp-agent.service
%config(noreplace) %attr(0600,root,root) %{_sysconfdir}/opennhrp-agent/agent.env
