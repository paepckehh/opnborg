package opnborg

import "encoding/xml"

// Opnsense
type Opnsense struct {
	XMLName xml.Name `xml:"opnsense"`
	Text    string   `xml:",chardata"`
	Theme   string   `xml:"theme"`
	Sysctl  struct {
		Text string `xml:",chardata"`
		Item []struct {
			Text    string `xml:",chardata"`
			Descr   string `xml:"descr"`
			Tunable string `xml:"tunable"`
			Value   string `xml:"value"`
		} `xml:"item"`
	} `xml:"sysctl"`
	System struct {
		Text             string `xml:",chardata"`
		Optimization     string `xml:"optimization"`
		Hostname         string `xml:"hostname"`
		Domain           string `xml:"domain"`
		Dnsallowoverride string `xml:"dnsallowoverride"`
		Group            struct {
			Text        string `xml:",chardata"`
			Name        string `xml:"name"`
			Description string `xml:"description"`
			Scope       string `xml:"scope"`
			Gid         string `xml:"gid"`
			Member      string `xml:"member"`
			Priv        string `xml:"priv"`
		} `xml:"group"`
		User []struct {
			Text           string   `xml:",chardata"`
			Name           string   `xml:"name"`
			Descr          string   `xml:"descr"`
			Scope          string   `xml:"scope"`
			Groupname      string   `xml:"groupname"`
			Password       string   `xml:"password"`
			Uid            string   `xml:"uid"`
			Priv           []string `xml:"priv"`
			Expires        string   `xml:"expires"`
			Authorizedkeys string   `xml:"authorizedkeys"`
			OtpSeed        string   `xml:"otp_seed"`
			Apikeys        struct {
				Text string `xml:",chardata"`
				Item struct {
					Text   string `xml:",chardata"`
					Key    string `xml:"key"`
					Secret string `xml:"secret"`
				} `xml:"item"`
			} `xml:"apikeys"`
		} `xml:"user"`
		Nextuid     string `xml:"nextuid"`
		Nextgid     string `xml:"nextgid"`
		Timezone    string `xml:"timezone"`
		Timeservers string `xml:"timeservers"`
		Webgui      struct {
			Text       string `xml:",chardata"`
			Protocol   string `xml:"protocol"`
			SslCertref string `xml:"ssl-certref"`
		} `xml:"webgui"`
		Disablenatreflection          string `xml:"disablenatreflection"`
		Usevirtualterminal            string `xml:"usevirtualterminal"`
		Disableconsolemenu            string `xml:"disableconsolemenu"`
		Disablevlanhwfilter           string `xml:"disablevlanhwfilter"`
		Disablechecksumoffloading     string `xml:"disablechecksumoffloading"`
		Disablesegmentationoffloading string `xml:"disablesegmentationoffloading"`
		Disablelargereceiveoffloading string `xml:"disablelargereceiveoffloading"`
		Ipv6allow                     string `xml:"ipv6allow"`
		PowerdAcMode                  string `xml:"powerd_ac_mode"`
		PowerdBatteryMode             string `xml:"powerd_battery_mode"`
		PowerdNormalMode              string `xml:"powerd_normal_mode"`
		Bogons                        struct {
			Text     string `xml:",chardata"`
			Interval string `xml:"interval"`
		} `xml:"bogons"`
		PfShareForward string `xml:"pf_share_forward"`
		LbUseSticky    string `xml:"lb_use_sticky"`
		Ssh            struct {
			Text  string `xml:",chardata"`
			Group string `xml:"group"`
		} `xml:"ssh"`
		Rrdbackup     string `xml:"rrdbackup"`
		Netflowbackup string `xml:"netflowbackup"`
		Firmware      struct {
			Text         string `xml:",chardata"`
			Version      string `xml:"version,attr"`
			Mirror       string `xml:"mirror"`
			Flavour      string `xml:"flavour"`
			Plugins      string `xml:"plugins"`
			Type         string `xml:"type"`
			Subscription string `xml:"subscription"`
			Reboot       string `xml:"reboot"`
		} `xml:"firmware"`
		Dnsserver string `xml:"dnsserver"`
	} `xml:"system"`
	Interfaces struct {
		Text string `xml:",chardata"`
		Lan  struct {
			Text      string `xml:",chardata"`
			Enable    string `xml:"enable"`
			If        string `xml:"if"`
			Ipaddr    string `xml:"ipaddr"`
			Subnet    string `xml:"subnet"`
			Ipaddrv6  string `xml:"ipaddrv6"`
			Subnetv6  string `xml:"subnetv6"`
			Media     string `xml:"media"`
			Mediaopt  string `xml:"mediaopt"`
			Gateway   string `xml:"gateway"`
			Gatewayv6 string `xml:"gatewayv6"`
			Descr     string `xml:"descr"`
		} `xml:"lan"`
		Lo0 struct {
			Text            string `xml:",chardata"`
			InternalDynamic string `xml:"internal_dynamic"`
			Descr           string `xml:"descr"`
			Enable          string `xml:"enable"`
			If              string `xml:"if"`
			Ipaddr          string `xml:"ipaddr"`
			Ipaddrv6        string `xml:"ipaddrv6"`
			Subnet          string `xml:"subnet"`
			Subnetv6        string `xml:"subnetv6"`
			Type            string `xml:"type"`
			Virtual         string `xml:"virtual"`
		} `xml:"lo0"`
		Wireguard struct {
			Text            string `xml:",chardata"`
			InternalDynamic string `xml:"internal_dynamic"`
			Descr           string `xml:"descr"`
			If              string `xml:"if"`
			Virtual         string `xml:"virtual"`
			Enable          string `xml:"enable"`
			Type            string `xml:"type"`
			Networks        string `xml:"networks"`
		} `xml:"wireguard"`
		Opt1 struct {
			Text  string `xml:",chardata"`
			Descr string `xml:"descr"`
			If    string `xml:"if"`
		} `xml:"opt1"`
	} `xml:"interfaces"`
	Dhcpd struct {
		Text string `xml:",chardata"`
		Lan  struct {
			Text  string `xml:",chardata"`
			Range struct {
				Text string `xml:",chardata"`
				From string `xml:"from"`
				To   string `xml:"to"`
			} `xml:"range"`
		} `xml:"lan"`
	} `xml:"dhcpd"`
	Snmpd struct {
		Text        string `xml:",chardata"`
		Syslocation string `xml:"syslocation"`
		Syscontact  string `xml:"syscontact"`
		Rocommunity string `xml:"rocommunity"`
	} `xml:"snmpd"`
	Nat struct {
		Text     string `xml:",chardata"`
		Outbound struct {
			Text string `xml:",chardata"`
			Mode string `xml:"mode"`
		} `xml:"outbound"`
	} `xml:"nat"`
	Filter struct {
		Text string `xml:",chardata"`
		Rule []struct {
			Text       string `xml:",chardata"`
			Type       string `xml:"type"`
			Ipprotocol string `xml:"ipprotocol"`
			Descr      string `xml:"descr"`
			Interface  string `xml:"interface"`
			Source     struct {
				Text    string `xml:",chardata"`
				Network string `xml:"network"`
			} `xml:"source"`
			Destination struct {
				Text string `xml:",chardata"`
				Any  string `xml:"any"`
			} `xml:"destination"`
		} `xml:"rule"`
	} `xml:"filter"`
	Rrd struct {
		Text   string `xml:",chardata"`
		Enable string `xml:"enable"`
	} `xml:"rrd"`
	Ntpd struct {
		Text   string `xml:",chardata"`
		Prefer string `xml:"prefer"`
	} `xml:"ntpd"`
	Revision struct {
		Text        string `xml:",chardata"`
		Username    string `xml:"username"`
		Time        string `xml:"time"`
		Description string `xml:"description"`
	} `xml:"revision"`
	OPNsense struct {
		Text     string `xml:",chardata"`
		DHCRelay struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
		} `xml:"DHCRelay"`
		Proxy struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text       string `xml:",chardata"`
				Enabled    string `xml:"enabled"`
				ErrorPages string `xml:"error_pages"`
				IcpPort    string `xml:"icpPort"`
				Logging    struct {
					Text   string `xml:",chardata"`
					Enable struct {
						Text      string `xml:",chardata"`
						AccessLog string `xml:"accessLog"`
						StoreLog  string `xml:"storeLog"`
					} `xml:"enable"`
					IgnoreLogACL string `xml:"ignoreLogACL"`
					Target       string `xml:"target"`
				} `xml:"logging"`
				AlternateDNSservers   string `xml:"alternateDNSservers"`
				ForwardedForHandling  string `xml:"forwardedForHandling"`
				UriWhitespaceHandling string `xml:"uriWhitespaceHandling"`
				EnablePinger          string `xml:"enablePinger"`
				UseViaHeader          string `xml:"useViaHeader"`
				SuppressVersion       string `xml:"suppressVersion"`
				Connecttimeout        string `xml:"connecttimeout"`
				VisibleEmail          string `xml:"VisibleEmail"`
				VisibleHostname       string `xml:"VisibleHostname"`
				Cache                 struct {
					Text  string `xml:",chardata"`
					Local struct {
						Text                      string `xml:",chardata"`
						Enabled                   string `xml:"enabled"`
						Directory                 string `xml:"directory"`
						CacheMem                  string `xml:"cache_mem"`
						MaximumObjectSize         string `xml:"maximum_object_size"`
						MaximumObjectSizeInMemory string `xml:"maximum_object_size_in_memory"`
						MemoryCacheMode           string `xml:"memory_cache_mode"`
						Size                      string `xml:"size"`
						L1                        string `xml:"l1"`
						L2                        string `xml:"l2"`
						CacheLinuxPackages        string `xml:"cache_linux_packages"`
						CacheWindowsUpdates       string `xml:"cache_windows_updates"`
					} `xml:"local"`
				} `xml:"cache"`
				Traffic struct {
					Text                       string `xml:",chardata"`
					Enabled                    string `xml:"enabled"`
					MaxDownloadSize            string `xml:"maxDownloadSize"`
					MaxUploadSize              string `xml:"maxUploadSize"`
					OverallBandwidthTrotteling string `xml:"OverallBandwidthTrotteling"`
					PerHostTrotteling          string `xml:"perHostTrotteling"`
				} `xml:"traffic"`
				Parentproxy struct {
					Text         string `xml:",chardata"`
					Enabled      string `xml:"enabled"`
					Host         string `xml:"host"`
					Enableauth   string `xml:"enableauth"`
					User         string `xml:"user"`
					Password     string `xml:"password"`
					Port         string `xml:"port"`
					Localdomains string `xml:"localdomains"`
					Localips     string `xml:"localips"`
				} `xml:"parentproxy"`
			} `xml:"general"`
			Forward struct {
				Text                      string `xml:",chardata"`
				Interfaces                string `xml:"interfaces"`
				Port                      string `xml:"port"`
				Sslbumpport               string `xml:"sslbumpport"`
				Sslbump                   string `xml:"sslbump"`
				Sslurlonly                string `xml:"sslurlonly"`
				Sslcertificate            string `xml:"sslcertificate"`
				Sslnobumpsites            string `xml:"sslnobumpsites"`
				SslCrtdStorageMaxSize     string `xml:"ssl_crtd_storage_max_size"`
				SslcrtdChildren           string `xml:"sslcrtd_children"`
				SnmpEnable                string `xml:"snmp_enable"`
				SnmpPort                  string `xml:"snmp_port"`
				SnmpPassword              string `xml:"snmp_password"`
				FtpInterfaces             string `xml:"ftpInterfaces"`
				FtpPort                   string `xml:"ftpPort"`
				FtpTransparentMode        string `xml:"ftpTransparentMode"`
				AddACLforInterfaceSubnets string `xml:"addACLforInterfaceSubnets"`
				TransparentMode           string `xml:"transparentMode"`
				Acl                       struct {
					Text           string `xml:",chardata"`
					AllowedSubnets string `xml:"allowedSubnets"`
					Unrestricted   string `xml:"unrestricted"`
					BannedHosts    string `xml:"bannedHosts"`
					WhiteList      string `xml:"whiteList"`
					BlackList      string `xml:"blackList"`
					Browser        string `xml:"browser"`
					MimeType       string `xml:"mimeType"`
					Googleapps     string `xml:"googleapps"`
					Youtube        string `xml:"youtube"`
					SafePorts      string `xml:"safePorts"`
					SslPorts       string `xml:"sslPorts"`
					RemoteACLs     struct {
						Text       string `xml:",chardata"`
						Blacklists string `xml:"blacklists"`
						UpdateCron string `xml:"UpdateCron"`
					} `xml:"remoteACLs"`
				} `xml:"acl"`
				Icap struct {
					Text           string `xml:",chardata"`
					Enable         string `xml:"enable"`
					RequestURL     string `xml:"RequestURL"`
					ResponseURL    string `xml:"ResponseURL"`
					SendClientIP   string `xml:"SendClientIP"`
					SendUsername   string `xml:"SendUsername"`
					EncodeUsername string `xml:"EncodeUsername"`
					UsernameHeader string `xml:"UsernameHeader"`
					EnablePreview  string `xml:"EnablePreview"`
					PreviewSize    string `xml:"PreviewSize"`
					OptionsTTL     string `xml:"OptionsTTL"`
					Exclude        string `xml:"exclude"`
				} `xml:"icap"`
				Authentication struct {
					Text             string `xml:",chardata"`
					Method           string `xml:"method"`
					AuthEnforceGroup string `xml:"authEnforceGroup"`
					Realm            string `xml:"realm"`
					Credentialsttl   string `xml:"credentialsttl"`
					Children         string `xml:"children"`
				} `xml:"authentication"`
			} `xml:"forward"`
			Pac        string `xml:"pac"`
			ErrorPages struct {
				Text     string `xml:",chardata"`
				Template string `xml:"template"`
			} `xml:"error_pages"`
		} `xml:"proxy"`
		Vnstat struct {
			Text    string `xml:",chardata"`
			General struct {
				Text      string `xml:",chardata"`
				Version   string `xml:"version,attr"`
				Enabled   string `xml:"enabled"`
				Interface string `xml:"interface"`
			} `xml:"general"`
		} `xml:"vnstat"`
		Bind struct {
			Text    string `xml:",chardata"`
			General struct {
				Text               string `xml:",chardata"`
				Version            string `xml:"version,attr"`
				Enabled            string `xml:"enabled"`
				Disablev6          string `xml:"disablev6"`
				Enablerpz          string `xml:"enablerpz"`
				Listenv4           string `xml:"listenv4"`
				Listenv6           string `xml:"listenv6"`
				Querysource        string `xml:"querysource"`
				Querysourcev6      string `xml:"querysourcev6"`
				Transfersource     string `xml:"transfersource"`
				Transfersourcev6   string `xml:"transfersourcev6"`
				Port               string `xml:"port"`
				Forwarders         string `xml:"forwarders"`
				Filteraaaav4       string `xml:"filteraaaav4"`
				Filteraaaav6       string `xml:"filteraaaav6"`
				Filteraaaaacl      string `xml:"filteraaaaacl"`
				Logsize            string `xml:"logsize"`
				GeneralLogLevel    string `xml:"general_log_level"`
				Maxcachesize       string `xml:"maxcachesize"`
				Recursion          string `xml:"recursion"`
				Allowtransfer      string `xml:"allowtransfer"`
				Allowquery         string `xml:"allowquery"`
				Dnssecvalidation   string `xml:"dnssecvalidation"`
				Hidehostname       string `xml:"hidehostname"`
				Hideversion        string `xml:"hideversion"`
				Disableprefetch    string `xml:"disableprefetch"`
				Enableratelimiting string `xml:"enableratelimiting"`
				Ratelimitcount     string `xml:"ratelimitcount"`
				Ratelimitexcept    string `xml:"ratelimitexcept"`
				Rndcalgo           string `xml:"rndcalgo"`
				Rndcsecret         string `xml:"rndcsecret"`
			} `xml:"general"`
			Dnsbl struct {
				Text                string `xml:",chardata"`
				Version             string `xml:"version,attr"`
				Enabled             string `xml:"enabled"`
				Type                string `xml:"type"`
				Whitelists          string `xml:"whitelists"`
				Forcesafegoogle     string `xml:"forcesafegoogle"`
				Forcesafeduckduckgo string `xml:"forcesafeduckduckgo"`
				Forcesafeyoutube    string `xml:"forcesafeyoutube"`
				Forcestrictbing     string `xml:"forcestrictbing"`
			} `xml:"dnsbl"`
			Domain struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Domains string `xml:"domains"`
			} `xml:"domain"`
			Record struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Records string `xml:"records"`
			} `xml:"record"`
			Acl struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Acls    string `xml:"acls"`
			} `xml:"acl"`
		} `xml:"bind"`
		HAProxy struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text            string `xml:",chardata"`
				Enabled         string `xml:"enabled"`
				GracefulStop    string `xml:"gracefulStop"`
				HardStopAfter   string `xml:"hardStopAfter"`
				CloseSpreadTime string `xml:"closeSpreadTime"`
				SeamlessReload  string `xml:"seamlessReload"`
				StoreOcsp       string `xml:"storeOcsp"`
				ShowIntro       string `xml:"showIntro"`
				Peers           struct {
					Text    string `xml:",chardata"`
					Enabled string `xml:"enabled"`
					Name1   string `xml:"name1"`
					Listen1 string `xml:"listen1"`
					Port1   string `xml:"port1"`
					Name2   string `xml:"name2"`
					Listen2 string `xml:"listen2"`
					Port2   string `xml:"port2"`
				} `xml:"peers"`
				Tuning struct {
					Text                           string `xml:",chardata"`
					Root                           string `xml:"root"`
					MaxConnections                 string `xml:"maxConnections"`
					Nbthread                       string `xml:"nbthread"`
					ResolversPrefer                string `xml:"resolversPrefer"`
					SslServerVerify                string `xml:"sslServerVerify"`
					MaxDHSize                      string `xml:"maxDHSize"`
					BufferSize                     string `xml:"bufferSize"`
					SpreadChecks                   string `xml:"spreadChecks"`
					BogusProxyEnabled              string `xml:"bogusProxyEnabled"`
					LuaMaxMem                      string `xml:"luaMaxMem"`
					CustomOptions                  string `xml:"customOptions"`
					OcspUpdateEnabled              string `xml:"ocspUpdateEnabled"`
					OcspUpdateMinDelay             string `xml:"ocspUpdateMinDelay"`
					OcspUpdateMaxDelay             string `xml:"ocspUpdateMaxDelay"`
					SslDefaultsEnabled             string `xml:"ssl_defaultsEnabled"`
					SslBindOptions                 string `xml:"ssl_bindOptions"`
					SslMinVersion                  string `xml:"ssl_minVersion"`
					SslMaxVersion                  string `xml:"ssl_maxVersion"`
					SslCipherList                  string `xml:"ssl_cipherList"`
					SslCipherSuites                string `xml:"ssl_cipherSuites"`
					H2InitialWindowSize            string `xml:"h2_initialWindowSize"`
					H2InitialWindowSizeOutgoing    string `xml:"h2_initialWindowSizeOutgoing"`
					H2InitialWindowSizeIncoming    string `xml:"h2_initialWindowSizeIncoming"`
					H2MaxConcurrentStreams         string `xml:"h2_maxConcurrentStreams"`
					H2MaxConcurrentStreamsOutgoing string `xml:"h2_maxConcurrentStreamsOutgoing"`
					H2MaxConcurrentStreamsIncoming string `xml:"h2_maxConcurrentStreamsIncoming"`
				} `xml:"tuning"`
				Defaults struct {
					Text                  string `xml:",chardata"`
					MaxConnections        string `xml:"maxConnections"`
					MaxConnectionsServers string `xml:"maxConnectionsServers"`
					TimeoutClient         string `xml:"timeoutClient"`
					TimeoutConnect        string `xml:"timeoutConnect"`
					TimeoutCheck          string `xml:"timeoutCheck"`
					TimeoutServer         string `xml:"timeoutServer"`
					Retries               string `xml:"retries"`
					Redispatch            string `xml:"redispatch"`
					InitAddr              string `xml:"init_addr"`
					CustomOptions         string `xml:"customOptions"`
				} `xml:"defaults"`
				Logging struct {
					Text     string `xml:",chardata"`
					Host     string `xml:"host"`
					Facility string `xml:"facility"`
					Level    string `xml:"level"`
					Length   string `xml:"length"`
				} `xml:"logging"`
				Stats struct {
					Text              string `xml:",chardata"`
					Enabled           string `xml:"enabled"`
					Port              string `xml:"port"`
					RemoteEnabled     string `xml:"remoteEnabled"`
					RemoteBind        string `xml:"remoteBind"`
					AuthEnabled       string `xml:"authEnabled"`
					Users             string `xml:"users"`
					AllowedUsers      string `xml:"allowedUsers"`
					AllowedGroups     string `xml:"allowedGroups"`
					CustomOptions     string `xml:"customOptions"`
					PrometheusEnabled string `xml:"prometheus_enabled"`
					PrometheusBind    string `xml:"prometheus_bind"`
					PrometheusPath    string `xml:"prometheus_path"`
				} `xml:"stats"`
				Cache struct {
					Text                string `xml:",chardata"`
					Enabled             string `xml:"enabled"`
					TotalMaxSize        string `xml:"totalMaxSize"`
					MaxAge              string `xml:"maxAge"`
					MaxObjectSize       string `xml:"maxObjectSize"`
					ProcessVary         string `xml:"processVary"`
					MaxSecondaryEntries string `xml:"maxSecondaryEntries"`
				} `xml:"cache"`
			} `xml:"general"`
			Frontends    string `xml:"frontends"`
			Backends     string `xml:"backends"`
			Servers      string `xml:"servers"`
			Healthchecks string `xml:"healthchecks"`
			Acls         string `xml:"acls"`
			Actions      string `xml:"actions"`
			Luas         string `xml:"luas"`
			Fcgis        string `xml:"fcgis"`
			Errorfiles   string `xml:"errorfiles"`
			Mapfiles     string `xml:"mapfiles"`
			Groups       string `xml:"groups"`
			Users        string `xml:"users"`
			Cpus         string `xml:"cpus"`
			Resolvers    string `xml:"resolvers"`
			Mailers      string `xml:"mailers"`
			Maintenance  struct {
				Text     string `xml:",chardata"`
				Cronjobs struct {
					Text               string `xml:",chardata"`
					SyncCerts          string `xml:"syncCerts"`
					SyncCertsCron      string `xml:"syncCertsCron"`
					UpdateOcsp         string `xml:"updateOcsp"`
					UpdateOcspCron     string `xml:"updateOcspCron"`
					ReloadService      string `xml:"reloadService"`
					ReloadServiceCron  string `xml:"reloadServiceCron"`
					RestartService     string `xml:"restartService"`
					RestartServiceCron string `xml:"restartServiceCron"`
				} `xml:"cronjobs"`
			} `xml:"maintenance"`
		} `xml:"HAProxy"`
		TrafficShaper struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			Pipes   string `xml:"pipes"`
			Queues  string `xml:"queues"`
			Rules   string `xml:"rules"`
		} `xml:"TrafficShaper"`
		Syslog struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text        string `xml:",chardata"`
				Enabled     string `xml:"enabled"`
				Loglocal    string `xml:"loglocal"`
				Maxpreserve string `xml:"maxpreserve"`
				Maxfilesize string `xml:"maxfilesize"`
			} `xml:"general"`
			Destinations string `xml:"destinations"`
		} `xml:"Syslog"`
		OpenVPN struct {
			Text       string `xml:",chardata"`
			Version    string `xml:"version,attr"`
			Overwrites string `xml:"Overwrites"`
			Instances  string `xml:"Instances"`
			StaticKeys string `xml:"StaticKeys"`
		} `xml:"OpenVPN"`
		OpenVPNExport struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			Servers string `xml:"servers"`
		} `xml:"OpenVPNExport"`
		Unboundplus struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text              string `xml:",chardata"`
				Enabled           string `xml:"enabled"`
				Port              string `xml:"port"`
				Stats             string `xml:"stats"`
				ActiveInterface   string `xml:"active_interface"`
				Dnssec            string `xml:"dnssec"`
				Dns64             string `xml:"dns64"`
				Dns64prefix       string `xml:"dns64prefix"`
				Noarecords        string `xml:"noarecords"`
				Regdhcp           string `xml:"regdhcp"`
				Regdhcpdomain     string `xml:"regdhcpdomain"`
				Regdhcpstatic     string `xml:"regdhcpstatic"`
				Noreglladdr6      string `xml:"noreglladdr6"`
				Noregrecords      string `xml:"noregrecords"`
				Txtsupport        string `xml:"txtsupport"`
				Cacheflush        string `xml:"cacheflush"`
				LocalZoneType     string `xml:"local_zone_type"`
				OutgoingInterface string `xml:"outgoing_interface"`
				EnableWpad        string `xml:"enable_wpad"`
			} `xml:"general"`
			Advanced struct {
				Text                      string `xml:",chardata"`
				Hideidentity              string `xml:"hideidentity"`
				Hideversion               string `xml:"hideversion"`
				Prefetch                  string `xml:"prefetch"`
				Prefetchkey               string `xml:"prefetchkey"`
				Dnssecstripped            string `xml:"dnssecstripped"`
				Aggressivensec            string `xml:"aggressivensec"`
				Serveexpired              string `xml:"serveexpired"`
				Serveexpiredreplyttl      string `xml:"serveexpiredreplyttl"`
				Serveexpiredttl           string `xml:"serveexpiredttl"`
				Serveexpiredttlreset      string `xml:"serveexpiredttlreset"`
				Serveexpiredclienttimeout string `xml:"serveexpiredclienttimeout"`
				Qnameminstrict            string `xml:"qnameminstrict"`
				Extendedstatistics        string `xml:"extendedstatistics"`
				Logqueries                string `xml:"logqueries"`
				Logreplies                string `xml:"logreplies"`
				Logtagqueryreply          string `xml:"logtagqueryreply"`
				Logservfail               string `xml:"logservfail"`
				Loglocalactions           string `xml:"loglocalactions"`
				Logverbosity              string `xml:"logverbosity"`
				Valloglevel               string `xml:"valloglevel"`
				Privatedomain             string `xml:"privatedomain"`
				Privateaddress            string `xml:"privateaddress"`
				Insecuredomain            string `xml:"insecuredomain"`
				Msgcachesize              string `xml:"msgcachesize"`
				Rrsetcachesize            string `xml:"rrsetcachesize"`
				Outgoingnumtcp            string `xml:"outgoingnumtcp"`
				Incomingnumtcp            string `xml:"incomingnumtcp"`
				Numqueriesperthread       string `xml:"numqueriesperthread"`
				Outgoingrange             string `xml:"outgoingrange"`
				Jostletimeout             string `xml:"jostletimeout"`
				Discardtimeout            string `xml:"discardtimeout"`
				Cachemaxttl               string `xml:"cachemaxttl"`
				Cachemaxnegativettl       string `xml:"cachemaxnegativettl"`
				Cacheminttl               string `xml:"cacheminttl"`
				Infrahostttl              string `xml:"infrahostttl"`
				Infrakeepprobing          string `xml:"infrakeepprobing"`
				Infracachenumhosts        string `xml:"infracachenumhosts"`
				Unwantedreplythreshold    string `xml:"unwantedreplythreshold"`
			} `xml:"advanced"`
			Acls struct {
				Text          string `xml:",chardata"`
				DefaultAction string `xml:"default_action"`
			} `xml:"acls"`
			Dnsbl struct {
				Text       string `xml:",chardata"`
				Enabled    string `xml:"enabled"`
				Safesearch string `xml:"safesearch"`
				Type       string `xml:"type"`
				Lists      string `xml:"lists"`
				Whitelists string `xml:"whitelists"`
				Blocklists string `xml:"blocklists"`
				Wildcards  string `xml:"wildcards"`
				Address    string `xml:"address"`
				Nxdomain   string `xml:"nxdomain"`
			} `xml:"dnsbl"`
			Forwarding struct {
				Text    string `xml:",chardata"`
				Enabled string `xml:"enabled"`
			} `xml:"forwarding"`
			Dots    string `xml:"dots"`
			Hosts   string `xml:"hosts"`
			Aliases string `xml:"aliases"`
			Domains string `xml:"domains"`
		} `xml:"unboundplus"`
		Captiveportal struct {
			Text      string `xml:",chardata"`
			Version   string `xml:"version,attr"`
			Zones     string `xml:"zones"`
			Templates string `xml:"templates"`
		} `xml:"captiveportal"`
		Kea struct {
			Text  string `xml:",chardata"`
			Dhcp4 struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				General struct {
					Text          string `xml:",chardata"`
					Enabled       string `xml:"enabled"`
					Interfaces    string `xml:"interfaces"`
					ValidLifetime string `xml:"valid_lifetime"`
					Fwrules       string `xml:"fwrules"`
				} `xml:"general"`
				Ha struct {
					Text           string `xml:",chardata"`
					Enabled        string `xml:"enabled"`
					ThisServerName string `xml:"this_server_name"`
				} `xml:"ha"`
				Subnets      string `xml:"subnets"`
				Reservations string `xml:"reservations"`
				HaPeers      string `xml:"ha_peers"`
			} `xml:"dhcp4"`
			CtrlAgent struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				General struct {
					Text     string `xml:",chardata"`
					Enabled  string `xml:"enabled"`
					HTTPHost string `xml:"http_host"`
					HTTPPort string `xml:"http_port"`
				} `xml:"general"`
			} `xml:"ctrl_agent"`
		} `xml:"Kea"`
		IDS struct {
			Text             string `xml:",chardata"`
			Version          string `xml:"version,attr"`
			Rules            string `xml:"rules"`
			Policies         string `xml:"policies"`
			UserDefinedRules string `xml:"userDefinedRules"`
			Files            string `xml:"files"`
			FileTags         string `xml:"fileTags"`
			General          struct {
				Text              string `xml:",chardata"`
				Enabled           string `xml:"enabled"`
				Ips               string `xml:"ips"`
				Promisc           string `xml:"promisc"`
				Interfaces        string `xml:"interfaces"`
				Homenet           string `xml:"homenet"`
				DefaultPacketSize string `xml:"defaultPacketSize"`
				UpdateCron        string `xml:"UpdateCron"`
				AlertLogrotate    string `xml:"AlertLogrotate"`
				AlertSaveLogs     string `xml:"AlertSaveLogs"`
				MPMAlgo           string `xml:"MPMAlgo"`
				Detect            struct {
					Text           string `xml:",chardata"`
					Profile        string `xml:"Profile"`
					ToclientGroups string `xml:"toclient_groups"`
					ToserverGroups string `xml:"toserver_groups"`
				} `xml:"detect"`
				Syslog     string `xml:"syslog"`
				SyslogEve  string `xml:"syslog_eve"`
				LogPayload string `xml:"LogPayload"`
				Verbosity  string `xml:"verbosity"`
			} `xml:"general"`
		} `xml:"IDS"`
		IPsec struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text                string `xml:",chardata"`
				Enabled             string `xml:"enabled"`
				PreferredOldsa      string `xml:"preferred_oldsa"`
				Disablevpnrules     string `xml:"disablevpnrules"`
				PassthroughNetworks string `xml:"passthrough_networks"`
			} `xml:"general"`
			Charon struct {
				Text               string `xml:",chardata"`
				MaxIkev1Exchanges  string `xml:"max_ikev1_exchanges"`
				Threads            string `xml:"threads"`
				IkesaTableSize     string `xml:"ikesa_table_size"`
				IkesaTableSegments string `xml:"ikesa_table_segments"`
				InitLimitHalfOpen  string `xml:"init_limit_half_open"`
				IgnoreAcquireTs    string `xml:"ignore_acquire_ts"`
				RetransmitTries    string `xml:"retransmit_tries"`
				RetransmitTimeout  string `xml:"retransmit_timeout"`
				RetransmitBase     string `xml:"retransmit_base"`
				RetransmitJitter   string `xml:"retransmit_jitter"`
				RetransmitLimit    string `xml:"retransmit_limit"`
				Syslog             struct {
					Text   string `xml:",chardata"`
					Daemon struct {
						Text     string `xml:",chardata"`
						IkeName  string `xml:"ike_name"`
						LogLevel string `xml:"log_level"`
						App      string `xml:"app"`
						Asn      string `xml:"asn"`
						Cfg      string `xml:"cfg"`
						Chd      string `xml:"chd"`
						Dmn      string `xml:"dmn"`
						Enc      string `xml:"enc"`
						Esp      string `xml:"esp"`
						Ike      string `xml:"ike"`
						Imc      string `xml:"imc"`
						Imv      string `xml:"imv"`
						Job      string `xml:"job"`
						Knl      string `xml:"knl"`
						Lib      string `xml:"lib"`
						Mgr      string `xml:"mgr"`
						Net      string `xml:"net"`
						Pts      string `xml:"pts"`
						Tls      string `xml:"tls"`
						Tnc      string `xml:"tnc"`
					} `xml:"daemon"`
				} `xml:"syslog"`
			} `xml:"charon"`
			KeyPairs      string `xml:"keyPairs"`
			PreSharedKeys string `xml:"preSharedKeys"`
		} `xml:"IPsec"`
		Swanctl struct {
			Text        string `xml:",chardata"`
			Version     string `xml:"version,attr"`
			Connections string `xml:"Connections"`
			Locals      string `xml:"locals"`
			Remotes     string `xml:"remotes"`
			Children    string `xml:"children"`
			Pools       string `xml:"Pools"`
			VTIs        string `xml:"VTIs"`
			SPDs        string `xml:"SPDs"`
		} `xml:"Swanctl"`
		Firewall struct {
			Text       string `xml:",chardata"`
			Lvtemplate struct {
				Text      string `xml:",chardata"`
				Version   string `xml:"version,attr"`
				Templates string `xml:"templates"`
			} `xml:"Lvtemplate"`
			Category struct {
				Text       string `xml:",chardata"`
				Version    string `xml:"version,attr"`
				Categories string `xml:"categories"`
			} `xml:"Category"`
			Alias struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Geoip   struct {
					Text string `xml:",chardata"`
					URL  string `xml:"url"`
				} `xml:"geoip"`
				Aliases string `xml:"aliases"`
			} `xml:"Alias"`
			Filter struct {
				Text      string `xml:",chardata"`
				Version   string `xml:"version,attr"`
				Rules     string `xml:"rules"`
				Snatrules string `xml:"snatrules"`
				Npt       string `xml:"npt"`
				Onetoone  string `xml:"onetoone"`
			} `xml:"Filter"`
		} `xml:"Firewall"`
		Netflow struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			Capture struct {
				Text       string `xml:",chardata"`
				Interfaces string `xml:"interfaces"`
				EgressOnly string `xml:"egress_only"`
				Version    string `xml:"version"`
				Targets    string `xml:"targets"`
			} `xml:"capture"`
			Collect struct {
				Text   string `xml:",chardata"`
				Enable string `xml:"enable"`
			} `xml:"collect"`
			ActiveTimeout   string `xml:"activeTimeout"`
			InactiveTimeout string `xml:"inactiveTimeout"`
		} `xml:"Netflow"`
		Wireguard struct {
			Text   string `xml:",chardata"`
			Server struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Servers struct {
					Text   string `xml:",chardata"`
					Server struct {
						Text          string `xml:",chardata"`
						Uuid          string `xml:"uuid,attr"`
						Enabled       string `xml:"enabled"`
						Name          string `xml:"name"`
						Instance      string `xml:"instance"`
						Pubkey        string `xml:"pubkey"`
						Privkey       string `xml:"privkey"`
						Port          string `xml:"port"`
						Mtu           string `xml:"mtu"`
						Dns           string `xml:"dns"`
						Tunneladdress string `xml:"tunneladdress"`
						Disableroutes string `xml:"disableroutes"`
						Gateway       string `xml:"gateway"`
						CarpDependOn  string `xml:"carp_depend_on"`
						Peers         string `xml:"peers"`
						Endpoint      string `xml:"endpoint"`
						PeerDns       string `xml:"peer_dns"`
					} `xml:"server"`
				} `xml:"servers"`
			} `xml:"server"`
			General struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Enabled string `xml:"enabled"`
			} `xml:"general"`
			Client struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
				Clients string `xml:"clients"`
			} `xml:"client"`
		} `xml:"wireguard"`
		QemuGuestAgent struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text         string `xml:",chardata"`
				Enabled      string `xml:"Enabled"`
				LogDebug     string `xml:"LogDebug"`
				DisabledRPCs string `xml:"DisabledRPCs"`
			} `xml:"general"`
		} `xml:"QemuGuestAgent"`
		Cron struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			Jobs    string `xml:"jobs"`
		} `xml:"cron"`
		Interfaces struct {
			Text      string `xml:",chardata"`
			Loopbacks struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
			} `xml:"loopbacks"`
			Neighbors struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
			} `xml:"neighbors"`
			Vxlans struct {
				Text    string `xml:",chardata"`
				Version string `xml:"version,attr"`
			} `xml:"vxlans"`
		} `xml:"Interfaces"`
		Gateways struct {
			Text        string `xml:",chardata"`
			Version     string `xml:"version,attr"`
			GatewayItem struct {
				Text           string `xml:",chardata"`
				Uuid           string `xml:"uuid,attr"`
				Disabled       string `xml:"disabled"`
				Name           string `xml:"name"`
				Descr          string `xml:"descr"`
				Interface      string `xml:"interface"`
				Ipprotocol     string `xml:"ipprotocol"`
				Gateway        string `xml:"gateway"`
				Defaultgw      string `xml:"defaultgw"`
				Fargw          string `xml:"fargw"`
				MonitorDisable string `xml:"monitor_disable"`
				MonitorNoroute string `xml:"monitor_noroute"`
				Monitor        string `xml:"monitor"`
				ForceDown      string `xml:"force_down"`
				Priority       string `xml:"priority"`
				Weight         string `xml:"weight"`
				Latencylow     string `xml:"latencylow"`
				Latencyhigh    string `xml:"latencyhigh"`
				Losslow        string `xml:"losslow"`
				Losshigh       string `xml:"losshigh"`
				Interval       string `xml:"interval"`
				TimePeriod     string `xml:"time_period"`
				LossInterval   string `xml:"loss_interval"`
				DataLength     string `xml:"data_length"`
			} `xml:"gateway_item"`
		} `xml:"Gateways"`
		Nginx struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text    string `xml:",chardata"`
				Enabled string `xml:"enabled"`
				BanTtl  string `xml:"ban_ttl"`
			} `xml:"general"`
			Webgui struct {
				Text          string `xml:",chardata"`
				Limitnetworks string `xml:"limitnetworks"`
			} `xml:"webgui"`
			HTTP struct {
				Text                      string `xml:",chardata"`
				Workerprocesses           string `xml:"workerprocesses"`
				Workerconnections         string `xml:"workerconnections"`
				Sendfile                  string `xml:"sendfile"`
				KeepaliveTimeout          string `xml:"keepalive_timeout"`
				ResetTimedout             string `xml:"reset_timedout"`
				DefaultType               string `xml:"default_type"`
				ServerNamesHashBucketSize string `xml:"server_names_hash_bucket_size"`
				ServerNamesHashMaxSize    string `xml:"server_names_hash_max_size"`
				BanResponse               string `xml:"ban_response"`
				LogPermBan                string `xml:"log_perm_ban"`
				BotsUa                    string `xml:"bots_ua"`
				HeadersMoreEnable         string `xml:"headers_more_enable"`
			} `xml:"http"`
		} `xml:"Nginx"`
		Monit struct {
			Text    string `xml:",chardata"`
			Version string `xml:"version,attr"`
			General struct {
				Text                      string `xml:",chardata"`
				Enabled                   string `xml:"enabled"`
				Interval                  string `xml:"interval"`
				Startdelay                string `xml:"startdelay"`
				Mailserver                string `xml:"mailserver"`
				Port                      string `xml:"port"`
				Username                  string `xml:"username"`
				Password                  string `xml:"password"`
				Ssl                       string `xml:"ssl"`
				Sslversion                string `xml:"sslversion"`
				Sslverify                 string `xml:"sslverify"`
				Logfile                   string `xml:"logfile"`
				Statefile                 string `xml:"statefile"`
				EventqueuePath            string `xml:"eventqueuePath"`
				EventqueueSlots           string `xml:"eventqueueSlots"`
				HttpdEnabled              string `xml:"httpdEnabled"`
				HttpdUsername             string `xml:"httpdUsername"`
				HttpdPassword             string `xml:"httpdPassword"`
				HttpdPort                 string `xml:"httpdPort"`
				HttpdAllow                string `xml:"httpdAllow"`
				MmonitUrl                 string `xml:"mmonitUrl"`
				MmonitTimeout             string `xml:"mmonitTimeout"`
				MmonitRegisterCredentials string `xml:"mmonitRegisterCredentials"`
			} `xml:"general"`
			Alert struct {
				Text        string `xml:",chardata"`
				Uuid        string `xml:"uuid,attr"`
				Enabled     string `xml:"enabled"`
				Recipient   string `xml:"recipient"`
				Noton       string `xml:"noton"`
				Events      string `xml:"events"`
				Format      string `xml:"format"`
				Reminder    string `xml:"reminder"`
				Description string `xml:"description"`
			} `xml:"alert"`
			Service []struct {
				Text         string `xml:",chardata"`
				Uuid         string `xml:"uuid,attr"`
				Enabled      string `xml:"enabled"`
				Name         string `xml:"name"`
				Description  string `xml:"description"`
				Type         string `xml:"type"`
				Pidfile      string `xml:"pidfile"`
				Match        string `xml:"match"`
				Path         string `xml:"path"`
				Timeout      string `xml:"timeout"`
				Starttimeout string `xml:"starttimeout"`
				Address      string `xml:"address"`
				Interface    string `xml:"interface"`
				Start        string `xml:"start"`
				Stop         string `xml:"stop"`
				Tests        string `xml:"tests"`
				Depends      string `xml:"depends"`
				Polltime     string `xml:"polltime"`
			} `xml:"service"`
			Test []struct {
				Text      string `xml:",chardata"`
				Uuid      string `xml:"uuid,attr"`
				Name      string `xml:"name"`
				Type      string `xml:"type"`
				Condition string `xml:"condition"`
				Action    string `xml:"action"`
				Path      string `xml:"path"`
			} `xml:"test"`
		} `xml:"monit"`
	} `xml:"OPNsense"`
	Openvpn string `xml:"openvpn"`
	Wol     struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
	} `xml:"wol"`
	Ifgroups struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
	} `xml:"ifgroups"`
	Hasync struct {
		Text            string `xml:",chardata"`
		Version         string `xml:"version,attr"`
		Disablepreempt  string `xml:"disablepreempt"`
		Disconnectppps  string `xml:"disconnectppps"`
		Pfsyncinterface string `xml:"pfsyncinterface"`
		Pfsyncpeerip    string `xml:"pfsyncpeerip"`
		Pfsyncversion   string `xml:"pfsyncversion"`
		Synchronizetoip string `xml:"synchronizetoip"`
		Username        string `xml:"username"`
		Password        string `xml:"password"`
		Syncitems       string `xml:"syncitems"`
	} `xml:"hasync"`
	Laggs struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Lagg    string `xml:"lagg"`
	} `xml:"laggs"`
	Gifs struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Gif     string `xml:"gif"`
	} `xml:"gifs"`
	Vlans struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Vlan    string `xml:"vlan"`
	} `xml:"vlans"`
	Gres struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Gre     string `xml:"gre"`
	} `xml:"gres"`
	Virtualip struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Vip     string `xml:"vip"`
	} `xml:"virtualip"`
	Staticroutes struct {
		Text    string `xml:",chardata"`
		Version string `xml:"version,attr"`
		Route   string `xml:"route"`
	} `xml:"staticroutes"`
	Deciso struct {
		Text  string `xml:",chardata"`
		Proxy struct {
			Text string `xml:",chardata"`
			ACL  struct {
				Text           string `xml:",chardata"`
				Version        string `xml:"version,attr"`
				Policies       string `xml:"policies"`
				CustomPolicies string `xml:"custom_policies"`
			} `xml:"ACL"`
		} `xml:"Proxy"`
	} `xml:"Deciso"`
	Bridges struct {
		Text    string `xml:",chardata"`
		Bridged string `xml:"bridged"`
	} `xml:"bridges"`
	Ppps struct {
		Text string `xml:",chardata"`
		Ppp  string `xml:"ppp"`
	} `xml:"ppps"`
	Wireless struct {
		Text  string `xml:",chardata"`
		Clone string `xml:"clone"`
	} `xml:"wireless"`
	Ca      string `xml:"ca"`
	Dhcpdv6 string `xml:"dhcpdv6"`
	Cert    []struct {
		Text  string `xml:",chardata"`
		Uuid  string `xml:"uuid,attr"`
		Refid string `xml:"refid"`
		Descr string `xml:"descr"`
		Caref string `xml:"caref"`
		Crt   string `xml:"crt"`
		Csr   string `xml:"csr"`
		Prv   string `xml:"prv"`
	} `xml:"cert"`
}
