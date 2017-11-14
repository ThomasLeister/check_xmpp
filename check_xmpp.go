package main

import (
    "fmt"
    "os"
    "net"
    "encoding/xml"
    "encoding/base64"
    "log"
    "io"
    "crypto/tls"
    "math/rand"
    "strings"
    "time"
    "io/ioutil"
    "flag"
)


/*
 * Host and login credentials
 */
var targethost string
var user string
var password string


/*
 * Helper functions
 */


/*
 * randString() creates random strings for IDs
 */
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
func randString(n int) string {
    b := make([]rune, n)
    for i := range b {
        b[i] = letterRunes[rand.Intn(len(letterRunes))]
    }
    return string(b)
}



/*
 * Message stanza type
 */

type MessageStanza struct {
    XMLName     xml.Name `xml:"jabber:client message"`
    From        string `xml:"from,attr"`
    Id          string `xml:"id,attr"`
    To          string `xml:"to,attr"`
    Type        string `xml:"type,attr"`
    InnerXML    []byte `xml:",innerxml"`
}


// Scan XML token stream to find next StartElement.
func nextStart(p *xml.Decoder) (xml.StartElement, error) {
	for {
		t, err := p.Token()
		if err != nil && err != io.EOF || t == nil {
			return xml.StartElement{}, err
		}
		switch t := t.(type) {
		case xml.StartElement:
            //log.Println("Next start element:", t.Name.Local, t.Name.Space)
			return t, nil
		}
	}
}



/*
 * Stream object which represents XMPP stream
 */
type XMPPStream struct {
    Host    string
    Login struct {
        Username string
        Password string
    }
    Conn    net.Conn
    Channels struct {
        Messages chan MessageStanza
    }
    Decoder     *xml.Decoder
}


/*
 * Establish basic XMPP stream. This includes:
 * - Establishing TCP connection
 * - Establishing XMPP stream
 * - Starting StartTLS encryption
 * - Authentication
 * - Resource binding
 */
func (stream *XMPPStream) Establish() {
    var err error

    stream.Channels.Messages = make(chan MessageStanza)

    // Lookup CNAME for SRV record
    _, addr, _ := net.LookupSRV("xmpp-client", "tcp", stream.Host)
    log.Println("Remote host:", addr[0].Target)
    host := addr[0].Target

    // Connect to XMPP PubSub server
    log.Println("Connecting to XMPP host", host, "...")
    stream.Conn, err = net.Dial("tcp", host + ":5222")
    if err != nil {
        log.Fatal(err)
    }

    // Set up XML decoder
    decoder := xml.NewDecoder(stream.Conn)

    // Open stream
    fmt.Fprintf(stream.Conn, "<?xml version='1.0'?><stream:stream " +
        "from='%s' " +
        "to='%s' " +
        "version='1.0' " +
        "xml:lang='en' "+
        "xmlns='jabber:client' " +
        "xmlns:stream='http://etherx.jabber.org/streams'>", user, targethost)

    // streamstart (<stream>)
    nextStart(decoder)

    // stream features
    nextStart(decoder)
    decoder.Skip()  // skip nested structures

    // Upgrade connection to STARTTLS connection
    var tlsconn *tls.Conn
    var tlsconfig = &tls.Config{ ServerName: targethost }
    tlsconn = tls.Client(stream.Conn, tlsconfig)

    // Send: Let's use starttls
    fmt.Fprintf(stream.Conn, "<starttls xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>")
    response_start, _:= nextStart(decoder)
    err = tlsconn.Handshake()
    if err != nil {
        log.Println("Could not initialize STARTTLS conn:", err)
    }
    stream.Conn = tlsconn

    // Re-open stream
    fmt.Fprintf(stream.Conn, "<?xml version='1.0'?><stream:stream " +
        "from='%s' " +
        "to='%s' " +
        "version='1.0' " +
        "xml:lang='en' "+
        "xmlns='jabber:client' " +
        "xmlns:stream='http://etherx.jabber.org/streams'>", user, targethost)

    // Re-Init decoder on new, secure connection
    decoder = xml.NewDecoder(stream.Conn)

    // Now there should be a new <stream> coming in
    nextStart(decoder)

    // Now we have an encrypted connection.
    // Get stream features once again
    nextStart(decoder)
    decoder.Skip() // skip nested structures


    /*
     * Authentication
     */

    // Let's do plain text auth, cuz this is easy
    // Plain authentication: send base64-encoded \x00 user \x00 password.
	raw := "\x00" + user + "\x00" + password
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(enc, []byte(raw))
    fmt.Fprintf(stream.Conn, "<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>%s</auth>", enc)

    response_start, _ = nextStart(decoder)
    if response_start.Name.Local == "failure" {
        terminate("CRITICAL", "Authentication failed.")
    } else {
        log.Println("Authentication successful")
    }


    /*
     * Open new stream after successful auth
     */
    fmt.Fprintf(stream.Conn, "<?xml version='1.0'?><stream:stream " +
        "from='%s' " +
        "to='%s' " +
        "version='1.0' " +
        "xml:lang='en' "+
        "xmlns='jabber:client' " +
        "xmlns:stream='http://etherx.jabber.org/streams'>", user, targethost)


    // new <stream> incoming
    nextStart(decoder)

    // stream features
    nextStart(decoder)


    /*
     * Bind resource
     */

    randres     := randString(10)
    bind_iq_id  := randString(10)
    fmt.Fprintf(stream.Conn,   "<iq id='%s' type='set'>" +
                                    "<bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'>" +
                                        "<resource>%s</resource>" +
                                    "</bind>" +
                                "</iq>", bind_iq_id, randres)


    /*
     * Set online state to be able to receive messages
     */
    fmt.Fprintf(stream.Conn, "<presence xml:lang='en'><show>online</show><status>Hey, I'm online.</status></presence>")

    /*
     * Stream is set up
     */
    stream.Decoder = decoder
    log.Println("XMPP stream is established")
}


/*
 * When stream is set up, loop through incoming stanzas
 * (iq, message, presence) and sort these stanzas into
 * seperate channels.
 * These channels can then be read by other functions
 */
func (stream *XMPPStream) Loop() {
    var element_type string

    for {
        incoming_start, err := nextStart(stream.Decoder)

        if err == nil {
            // Check element name and namespace of incoming element block
            element_type = incoming_start.Name.Space + " " + incoming_start.Name.Local

            switch element_type {
            case "jabber:client message":
                msgstanza := MessageStanza{}
                err := stream.Decoder.DecodeElement(&msgstanza, &incoming_start)
                if err != nil {
                    log.Println("Failed to parse incoming <message> stanza:", err)
                } else {
                    stream.Channels.Messages <- msgstanza
                }

            default:
                // We don't care about other stanzas. Ignore them.
                stream.Decoder.Skip()
            }
        } else {
            log.Fatal("Received invalid XML start token:", err)
        }
    }
}



func terminate(status string, message string) {
    // os.Exit(0) // ok
    // os.Exit(1) // warning
    // os.Exit(2) // critical
    // os.Exit(3) // unknown

    fmt.Println(status, "-", message)

    switch(status) {
    case "OK":
        os.Exit(0)
    case "WARNING":
        os.Exit(1)
    case "CRITICAL":
        os.Exit(2)
    default:
        os.Exit(3)
    }
}



/*****************************************************
 * Main function of this whole stuff
 *****************************************************/

func main() {
    var flag_debugging = flag.Bool("debug", false, "Enable debugging output")
    var flag_timeout = flag.Duration("timeout", 5 * time.Second, "Timeout")
    var flag_userid = flag.String("userid", "user@server.tld", "XMPP ID e.g. user@server.tld")
    var flag_password = flag.String("password", "mypassword", "Password for userid")

    // Read command line flags
    flag.Parse()

    // Disable debug logging
    if *flag_debugging == false {
        log.SetOutput(ioutil.Discard)
    }


    /*
     * Filter and save login credentials
     */
    password = *flag_password
    userid_split := strings.Split(*flag_userid, "@")
    if len(userid_split) < 2 {
        terminate("CRITICAL", "Please specify fully qualified user ID such as 'user@server.tld'")
    }
    user = userid_split[0]
    targethost = userid_split[1]

    // Start everything
    log.Println("XMPP check starting ... ")


    /*
     * Implements timeout. Process must not take too long.
     */
    go func() {
        time.Sleep(*flag_timeout)
        terminate("CRITICAL", "Timeout")
    }()


    // If something writes into quit channel, program will be quit.
    quit := make(chan bool)


    /*
     * Establish XMPP stream
     */
    // Create new stream object
    xmppstream := &XMPPStream{ Host: targethost }
    xmppstream.Login.Username = user
    xmppstream.Login.Password = password
    xmppstream.Establish()
    defer xmppstream.Conn.Close()

    // Start stream loop
    go xmppstream.Loop()

    /*
     * Send a message to myself
     */
    log.Println("Sending message ...")
    fmt.Fprintf(xmppstream.Conn,    "<message from='%s' to='%s'>" +
                                        "<body>Check</body>" +
                                    "</message>", *flag_userid, *flag_userid)


    /*
     * Wait until message is received
     */
    go func() {
        log.Println("Waiting for message to arrive ...")

        msgstanza := <- xmppstream.Channels.Messages

        if strings.Contains(msgstanza.From, *flag_userid) {
            // Exit with "ok" code
            log.Println("Received message")
            terminate("OK", "XMPP server is okay.")
        }
    }()


    // Quit program  if something is written to "quit" channel
    _ = <- quit
}
