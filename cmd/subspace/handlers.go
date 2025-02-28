package main

import (
	"bytes"
	"fmt"
	"image/png"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/crewjam/saml/samlsp"
	"github.com/julienschmidt/httprouter"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	qrcode "github.com/skip2/go-qrcode"
)

var (
	validEmail         = regexp.MustCompile(`^[ -~]+@[ -~]+$`)
	validPassword      = regexp.MustCompile(`^[ -~]{6,200}$`)
	validString        = regexp.MustCompile(`^[ -~]{1,200}$`)
	maxProfilesPerUser = 4
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Handles the sign in part separately from the SAML
func ssoHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	session, err := samlSP.Session.GetSession(r)
	if session != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err == samlsp.ErrNoSession {
		logger.Debugf("SSO: HandleStartAuthFlow")
		samlSP.HandleStartAuthFlow(w, r)
		return
	}

	logger.Debugf("SSO: unable to get session")
	samlSP.OnError(w, r, err)
	return
}

// Handles the SAML part separately from sign in
func samlHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if samlSP == nil {
		logger.Warnf("SAML is not configured")
		http.NotFound(w, r)
		return
	}
	logger.Debugf("SSO: samlSP.ServeHTTP")
	samlSP.ServeHTTP(w, r)
}

func wireguardQRConfigHandler(w *Web) {
	profile, err := config.FindProfile(w.ps.ByName("profile"))
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin && profile.UserID != w.User.ID {
		Error(w.w, fmt.Errorf("failed to view config: permission denied"))
		return
	}

	b, err := ioutil.ReadFile(profile.WireGuardConfigPath())
	if err != nil {
		Error(w.w, err)
		return
	}

	img, err := qrcode.Encode(string(b), qrcode.Medium, 256)
	if err != nil {
		Error(w.w, err)
		return
	}

	w.w.Header().Set("Content-Type", "image/png")
	w.w.Header().Set("Content-Length", fmt.Sprintf("%d", len(img)))
	if _, err := w.w.Write(img); err != nil {
		Error(w.w, err)
		return
	}
}

func wireguardConfigHandler(w *Web) {
	profile, err := config.FindProfile(w.ps.ByName("profile"))
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin && profile.UserID != w.User.ID {
		Error(w.w, fmt.Errorf("failed to view config: permission denied"))
		return
	}

	b, err := ioutil.ReadFile(profile.WireGuardConfigPath())
	if err != nil {
		Error(w.w, err)
		return
	}

	w.w.Header().Set("Content-Disposition", "attachment; filename="+profile.WireGuardConfigName())
	w.w.Header().Set("Content-Type", "application/x-wireguard-profile")
	w.w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
	if _, err := w.w.Write(b); err != nil {
		Error(w.w, err)
		return
	}
}

func configureHandler(w *Web) {
	if config.FindInfo().Configured {
		w.Redirect("/?error=configured")
		return
	}

	if w.r.Method == "GET" {
		w.HTML()
		return
	}

	email := strings.ToLower(strings.TrimSpace(w.r.FormValue("email")))
	emailConfirm := strings.ToLower(strings.TrimSpace(w.r.FormValue("email_confirm")))
	password := w.r.FormValue("password")

	if !validEmail.MatchString(email) || !validPassword.MatchString(password) || email != emailConfirm {
		w.Redirect("/configure?error=invalid")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		w.Redirect("/forgot?error=bcrypt")
		return
	}
	config.UpdateInfo(func(i *Info) error {
		i.Email = email
		i.Password = hashedPassword
		i.Configured = true
		return nil
	})

	if err := w.SigninSession(true, ""); err != nil {
		Error(w.w, err)
		return
	}
	w.Redirect("/settings?success=configured")
}

func forgotHandler(w *Web) {
	if w.r.Method == "GET" {
		w.HTML()
		return
	}

	email := strings.ToLower(strings.TrimSpace(w.r.FormValue("email")))
	secret := w.r.FormValue("secret")
	password := w.r.FormValue("password")

	if email != "" && !validEmail.MatchString(email) {
		w.Redirect("/forgot?error=invalid")
		return
	}
	if secret != "" && !validString.MatchString(secret) {
		w.Redirect("/forgot?error=invalid")
		return
	}
	if email != "" && secret != "" && !validPassword.MatchString(password) {
		w.Redirect("/forgot?error=invalid&email=%s&secret=%s", email, secret)
		return
	}

	if email != config.FindInfo().Email {
		w.Redirect("/forgot?error=invalid")
		return
	}

	if secret == "" {
		secret = config.FindInfo().Secret
		if secret == "" {
			secret = RandomString(32)
			config.UpdateInfo(func(i *Info) error {
				if i.Secret == "" {
					i.Secret = secret
				}
				return nil
			})
		}

		go func() {
			if err := mailer.Forgot(email, secret); err != nil {
				logger.Error(err)
			}
		}()

		w.Redirect("/forgot?success=forgot")
		return
	}

	if secret != config.FindInfo().Secret {
		w.Redirect("/forgot?error=invalid")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		w.Redirect("/forgot?error=bcrypt")
		return
	}
	config.UpdateInfo(func(i *Info) error {
		i.Password = hashedPassword
		i.Secret = ""
		return nil
	})

	if err := w.SigninSession(true, ""); err != nil {
		Error(w.w, err)
		return
	}
	w.Redirect("/")
}

func signoutHandler(w *Web) {
	w.SignoutSession()
	w.Redirect("/signin")
}

func signinHandler(w *Web) {
	if w.r.Method == "GET" {
		w.HTML()
		return
	}

	email := strings.ToLower(strings.TrimSpace(w.r.FormValue("email")))
	password := w.r.FormValue("password")
	passcode := w.r.FormValue("totp")

	if email != config.FindInfo().Email {
		w.Redirect("/signin?error=invalid")
		return
	}

	if err := bcrypt.CompareHashAndPassword(config.FindInfo().Password, []byte(password)); err != nil {
		w.Redirect("/signin?error=invalid")
		return
	}

	if config.FindInfo().TotpKey != "" && !totp.Validate(passcode, config.FindInfo().TotpKey) {
		// Totp has been configured and the provided code doesn't match
		w.Redirect("/signin?error=invalid")
		return
	}

	if err := w.SigninSession(true, ""); err != nil {
		Error(w.w, err)
		return
	}

	w.Redirect("/")
}

func totpQRHandler(w *Web) {
	if !w.Admin {
		Error(w.w, fmt.Errorf("failed to view config: permission denied"))
		return
	}

	if config.Info.TotpKey != "" {
		// TOTP is already configured, don't allow the current one to be leaked
		w.Redirect("/")
		return
	}

	var buf bytes.Buffer
	img, err := tempTotpKey.Image(200, 200)
	if err != nil {
		Error(w.w, err)
		return
	}

	png.Encode(&buf, img)

	w.w.Header().Set("Content-Type", "image/png")
	w.w.Header().Set("Content-Length", fmt.Sprintf("%d", len(buf.Bytes())))
	if _, err := w.w.Write(buf.Bytes()); err != nil {
		Error(w.w, err)
		return
	}

}

func userEditHandler(w *Web) {
	userID := w.ps.ByName("user")
	if userID == "" {
		userID = w.r.FormValue("user")
	}
	user, err := config.FindUser(userID)
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin {
		Error(w.w, fmt.Errorf("failed to edit user: permission denied"))
		return
	}

	if w.r.Method == "GET" {
		w.TargetUser = user
		w.Profiles = config.ListProfilesByUser(user.ID)
		w.HTML()
		return
	}

	if w.User.ID == user.ID {
		w.Redirect("/user/edit/%s", user.ID)
		return
	}

	admin := w.r.FormValue("admin") == "yes"

	config.UpdateUser(user.ID, func(u *User) error {
		u.Admin = admin
		return nil
	})

	w.Redirect("/user/edit/%s?success=edituser", user.ID)
}

func userDeleteHandler(w *Web) {
	userID := w.ps.ByName("user")
	if userID == "" {
		userID = w.r.FormValue("user")
	}
	user, err := config.FindUser(userID)
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin {
		Error(w.w, fmt.Errorf("failed to delete user: permission denied"))
		return
	}
	if w.User.ID == user.ID {
		w.Redirect("/user/edit/%s?error=deleteuser", user.ID)
		return
	}

	if w.r.Method == "GET" {
		w.TargetUser = user
		w.HTML()
		return
	}

	for _, profile := range config.ListProfilesByUser(user.ID) {
		if err := deleteProfile(profile); err != nil {
			logger.Errorf("delete profile failed: %s", err)
			w.Redirect("/profile/delete?error=deleteprofile")
			return
		}
	}

	if err := config.DeleteUser(user.ID); err != nil {
		Error(w.w, err)
		return
	}
	w.Redirect("/?success=deleteuser")
}

func profileAddHandler(w *Web) {
	if !w.Admin && w.User.ID == "" {
		http.NotFound(w.w, w.r)
		return
	}

	name := strings.TrimSpace(w.r.FormValue("name"))
	platform := strings.TrimSpace(w.r.FormValue("platform"))
	ipspeer := strings.TrimSpace(w.r.FormValue("ipspeer"))
	allowedips := strings.TrimSpace(w.r.FormValue("allowedips"))
	admin := w.r.FormValue("admin") == "yes"

	if platform == "" {
		platform = "other"
	}
	if ipspeer == "no" {
		ipspeer = "127.0.1.1/32"
	}
	if allowedips == "no" {
		allowedips = "10.0.0.0/16,10.2.0.0/16,10.3.0.0/16,10.4.0.0/16,192.168.160.0/24,192.168.178.0/23,192.168.78.0/24,10.233.0.0/18"
	}
	if name == "" {
		w.Redirect("/?error=profilename")
		return
	}

	var userID string
	if admin {
		userID = ""
	} else {
		userID = w.User.ID
	}

	//if len(config.ListProfiles()) >= maxProfiles {
	// Check whether there is a room to assign new IP address or not.
	firstFreeID := FindFirstFreeID(config.Profiles)
	logger.Debugf("firstFreeID for generateIPAddr: %v", firstFreeID)
	if _, _, err := wgConfig.generateIPAddr(firstFreeID); err != nil {
		w.Redirect("/?error=addprofile")
		return
	}
	if !admin {
		logger.Debugf("You not admin!!!")
		if len(config.ListProfilesByUser(userID)) >= maxProfilesPerUser {
			w.Redirect("/?error=addprofile")
			return
		}
	}
	profile, err := config.AddProfile(int(firstFreeID), userID, name, platform, ipspeer, allowedips)
	if err != nil {
		logger.Debugf("Error add Profile: %v", err)
		logger.Warn(err)
		w.Redirect("/?error=addprofile")
		return
	}

	ipv4Gw := "10.99.80.1"
	if gw := getEnv("SUBSPACE_IPV4_GW", "nil"); gw != "nil" {
		ipv4Gw = gw
	}
	ipv6Gw := "fd00::10:97:1"
	if gw := getEnv("SUBSPACE_IPV6_GW", "nil"); gw != "nil" {
		ipv6Gw = gw
	}
	listenport := "51820"
	if port := getEnv("SUBSPACE_LISTENPORT", "nil"); port != "nil" {
		listenport = port
	}
	upstreamdns := "1.1.1.1"
	if udns := getEnv("SUBSPACE_NAMESERVER_IPv4", "nil"); udns != "nil" {
		upstreamdns = udns
	}
	endpointHost := httpHost
	if eh := getEnv("SUBSPACE_ENDPOINT_HOST", "nil"); eh != "nil" {
		endpointHost = eh
	}
	// disable overriding allowedips = "0.0.0.0/0, ::/0"
	if ips := getEnv("SUBSPACE_ALLOWED_IPS", "nil"); ips != "nil" {
		allowedips = ips
	}
	upstreamdns6 := "2606:4700:4700::1111"
	if udns6 := getEnv("SUBSPACE_NAMESERVER_IPv6", "nil"); udns6 != "nil" {
		upstreamdns6 = udns6
	}
	ipv4Enabled := true
	if enable := getEnv("SUBSPACE_IPV4_NAT_ENABLED", "1"); enable == "0" {
		ipv4Enabled = false
	}
	ipv6Enabled := true
	if enable := getEnv("SUBSPACE_IPV6_NAT_ENABLED", "1"); enable == "0" {
		ipv6Enabled = false
	}
	disableDNS := false
	if shouldDisableDNS := getEnv("SUBSPACE_DISABLE_DNS", "0"); shouldDisableDNS == "1" {
		disableDNS = true
	}
	ipv4Addr, ipv6Addr, err := wgConfig.generateIPAddr(uint32(profile.Number)) // wgConfig.generateIPAddr(FindFirstFreeID(config.Profiles))
	if err != nil {
		logger.Errorf("Failed to generate IP addres for Profile %s: %v", profile.ID, err)
		w.Redirect("/?error=addprofile")
		return
	}
	ipv4Gw = wgConfig.gatewayIPv4.String()
	ipv6Gw = wgConfig.gatewayIPv6.String()
	ipv4CIDR := wgConfig.networkIPv4.String()
	ipv6CIDR := wgConfig.networkIPv4.String()
	script := `
cd {{$.Datadir}}/wireguard
wg_private_key="$(wg genkey)"
wg_public_key="$(echo $wg_private_key | wg pubkey)"

wg set wg0 peer ${wg_public_key} allowed-ips {{if .Ipv4Enabled}}{{$.IPv4Addr}}/32{{end}}{{if .Ipv6Enabled}}{{if .Ipv4Enabled}},{{end}}{{$.IPv6Addr}}/128{{end}}

cat <<WGPEER >peers/{{$.Profile.ID}}.conf
[Peer]
PublicKey = ${wg_public_key}
AllowedIPs = {{if .Ipv4Enabled}}{{$.IPv4Addr}}/32{{end}}{{if .Ipv6Enabled}}{{if .Ipv4Enabled}},{{end}}{{$.IPv6Addr}}/128{{end}}
WGPEER

cat <<WGCLIENT >clients/{{$.Profile.ID}}.conf
[Interface]
PrivateKey = ${wg_private_key}
{{- if not .DisableDNS }}
DNS = {{if .Ipv4Enabled}}{{$.UpstreamDNS}}{{end}}{{if .Ipv6Enabled}}{{if .Ipv4Enabled}},{{end}}{{$.UpstreamDNS6}}{{end}}
{{- end }}
Address = {{if .Ipv4Enabled}}{{$.IPv4Addr}}/32{{end}}{{if .Ipv6Enabled}}{{if .Ipv4Enabled}},{{end}}{{$.IPv6Addr}}/128{{end}}

[Peer]
PublicKey = $(cat server.public)

Endpoint = {{$.EndpointHost}}:{{$.Listenport}}
AllowedIPs = {{$.AllowedIPS}}
WGCLIENT
`
	_, err = bash(script, struct {
		Profile      Profile
		EndpointHost string
		Datadir      string
		IPv4Gw       string
		IPv6Gw       string
		IPv4Addr     string
		IPv6Addr     string
		IPv4Cidr     string
		IPv6Cidr     string
		Listenport   string
		UpstreamDNS  string
		UpstreamDNS6 string
		AllowedIPS   string
		Ipv4Enabled  bool
		Ipv6Enabled  bool
		DisableDNS   bool
	}{
		profile,
		endpointHost,
		datadir,
		ipv4Gw,
		ipv6Gw,
		ipv4Addr.String(),
		ipv6Addr.String(),
		ipv4CIDR,
		ipv6CIDR,
		listenport,
		upstreamdns,
		upstreamdns6,
		allowedips,
		ipv4Enabled,
		ipv6Enabled,
		disableDNS,
	})
	if err != nil {
		logger.Debugf("Error bash execute, see /tmp/error.txt")
		logger.Warn(err)
		f, _ := os.Create("/tmp/error.txt")
		errstr := fmt.Sprintln(err)
		f.WriteString(errstr)
		w.Redirect("/?error=addprofile")
		return
	}

	w.Redirect("/profile/connect/%s?success=addprofile", profile.ID)
}

func profileConnectHandler(w *Web) {
	profile, err := config.FindProfile(w.ps.ByName("profile"))
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin && profile.UserID != w.User.ID {
		Error(w.w, fmt.Errorf("failed to view profile: permission denied"))
		return
	}
	w.Profile = profile
	w.HTML()
}

func profileDeleteHandler(w *Web) {
	profileID := w.ps.ByName("profile")
	if profileID == "" {
		profileID = w.r.FormValue("profile")
	}
	profile, err := config.FindProfile(profileID)
	if err != nil {
		http.NotFound(w.w, w.r)
		return
	}
	if !w.Admin && profile.UserID != w.User.ID {
		Error(w.w, fmt.Errorf("failed to delete profile: permission denied"))
		return
	}

	if w.r.Method == "GET" {
		w.Profile = profile
		w.HTML()
		return
	}
	if err := deleteProfile(profile); err != nil {
		logger.Errorf("delete profile failed: %s", err)
		w.Redirect("/profile/delete?error=deleteprofile")
		return
	}
	if w.Admin {
		w.Redirect("/user/edit/%s?success=deleteprofile", profile.UserID)
		return
	}
	w.Redirect("/?success=deleteprofile")
}

func indexHandler(w *Web) {
	if w.User.ID != "" {
		w.TargetProfiles = config.ListProfilesByUser(w.User.ID)
	}
	if w.Admin {
		w.Profiles = config.ListProfilesByUser("")
		w.Users = config.ListUsers()
	} else {
		w.Profiles = config.ListProfilesByUser(w.User.ID)
	}
	w.HTML()
}

func settingsHandler(w *Web) {
	if !w.Admin {
		Error(w.w, fmt.Errorf("settings: permission denied"))
		return
	}

	if w.r.Method == "GET" {
		w.HTML()
		return
	}

	email := strings.ToLower(strings.TrimSpace(w.r.FormValue("email")))
	samlMetadata := strings.TrimSpace(w.r.FormValue("saml_metadata"))

	currentPassword := w.r.FormValue("current_password")
	newPassword := w.r.FormValue("new_password")

	resetTotp := w.r.FormValue("reset_totp")
	totpCode := w.r.FormValue("totp_code")

	config.UpdateInfo(func(i *Info) error {
		i.SAML.IDPMetadata = samlMetadata
		i.Email = email
		return nil
	})

	// Configure SAML if metadata is present.
	if len(samlMetadata) > 0 {
		if err := configureSAML(); err != nil {
			logger.Warnf("configuring SAML failed: %s", err)
			w.Redirect("/settings?error=saml")
		}
	} else {
		samlSP = nil
	}

	if currentPassword != "" || newPassword != "" {
		if !validPassword.MatchString(newPassword) {
			w.Redirect("/settings?error=invalid")
			return
		}

		if err := bcrypt.CompareHashAndPassword(config.FindInfo().Password, []byte(currentPassword)); err != nil {
			w.Redirect("/settings?error=invalid")
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			w.Redirect("/settings?error=bcrypt")
			return
		}

		config.UpdateInfo(func(i *Info) error {
			i.Password = hashedPassword
			return nil
		})
	}

	if resetTotp == "true" {
		err := config.ResetTotp()
		if err != nil {
			w.Redirect("/settings?error=totp")
			return
		}

		w.Redirect("/settings?success=totp")
		return
	}

	if config.Info.TotpKey == "" && totpCode != "" {
		if !totp.Validate(totpCode, tempTotpKey.Secret()) {
			w.Redirect("/settings?error=totp")
			return
		}
		config.Info.TotpKey = tempTotpKey.Secret()
		config.save()
	}

	w.Redirect("/settings?success=settings")
}

func helpHandler(w *Web) {
	w.HTML()
}

//
// Helpers
//
func deleteProfile(profile Profile) error {
	script := `
# WireGuard
cd {{$.Datadir}}/wireguard
peerid=$(cat peers/{{$.Profile.ID}}.conf | awk '/PublicKey/ { printf("%s", $3) }' )
wg set wg0 peer $peerid remove
rm peers/{{$.Profile.ID}}.conf
rm clients/{{$.Profile.ID}}.conf
`
	output, err := bash(script, struct {
		Datadir string
		Profile Profile
	}{
		datadir,
		profile,
	})
	if err != nil {
		return fmt.Errorf("delete profile failed %s %s", err, output)
	}
	return config.DeleteProfile(profile.ID)
}
