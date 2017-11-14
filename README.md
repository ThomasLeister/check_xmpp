check_xmpp Nagios / Icinga2 XMPP server check
======================================================

check_xmpp is a Nagios / Icinga2 check plugin written in Golang. It is capable of checking the connection to your XMPP server, authentication and message delivery. A valid XMPP account on the target server is needed for the plugin to work.

---
**!!! This plugin is only compatible with STARTTLS-enabled servers and plain authentication !!!**

*... which most servers do offer.*

---

## Usage

```
./check_xmpp -timeout 5s -userid user@server.tld -password mypassword
```

You can add a ```-debug``` flag if things do not work.

## Download / Compiling

* Download latest binary from ["Releases" section](https://github.com/ThomasLeister/check_xmpp/releases) on GitHub
* ... or compile yourself by [installing Golang](https://golang.org/doc/install) and running "build.sh"

## Installation

1. Copy binary to ```/usr/local/nagios-plugins/check_xmpp```

2. Register check command in ```/etc/icinga2/conf.d/commands.conf```:
```    
object CheckCommand "xmpp_client" {
    import "plugin-check-command"

    command = ["/usr/local/nagios-plugins/check_xmpp" ]
    arguments = {
        "-timeout" = "$timeout$"
        "-userid" = "$userid$"
        "-password" = "$password$"
    }
}
```

3. Register check in ```/etc/icinga2/conf.d/services.conf```
```
object Service "xmpp_client" {
    host_name = "xmpphost"
    check_command = "xmpp_client"
    vars.userid = "checkuser@server.tld"
    vars.password = "checkuserpassword"
    vars.timeout = "5s"
    check_interval = 2m
    retry_interval = 1m
}
```

4. Reload Icinga2: ```systemctl reload icinga2```


## Few words about the code

This is no professional coding work done by a long-term Go developer. Exception and error checks are missing in various places and code structure could definitely by cleaner. If you are more experienced and you have suggestions for nicer, cleaner code, feel free to contribute to this repository. :-)
