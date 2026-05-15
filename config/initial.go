package config

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/phuslu/log"

	"github.com/yaling888/quirktiva/common/convert"
	C "github.com/yaling888/quirktiva/constant"
)

func downloadMMDB(path string) (err error) {
	resp, err := doGet("geoip", "Country.mmdb")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)

	return err
}

func initMMDB() error {
	return nil
}

func downloadGeoSite(path string) (err error) {
	resp, err := doGet("geosite", "geosite.dat")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)

	return err
}

func initGeoSite() error {
	return nil
}

// Init prepare necessary files
func Init(dir string) error {
	// initial homedir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return fmt.Errorf("can't create config directory %s: %w", dir, err)
		}
	}

	// initial config.yaml
	if _, err := os.Stat(C.Path.Config()); os.IsNotExist(err) {
		log.Info().Msg("[Config] can't find config, create a initial config file")
		f, err := os.OpenFile(C.Path.Config(), os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("can't create file %s: %w", C.Path.Config(), err)
		}
		_, _ = f.Write([]byte(`mixed-port: 7890`))
		_ = f.Close()
	}

	// initial mmdb
	if err := initMMDB(); err != nil {
		return fmt.Errorf("can't initial MMDB: %w", err)
	}

	// initial GeoSite
	if err := initGeoSite(); err != nil {
		return fmt.Errorf("can't initial GeoSite: %w", err)
	}
	return nil
}

func doGet(name, file string) (resp *http.Response, err error) {
	var (
		req     *http.Request
		mirrors = []string{
			"https://raw.githubusercontent.com/yaling888/%s/release/%s",
			"https://cdn.jsdelivr.net/gh/yaling888/%s@release/%s",
			"https://gcore.jsdelivr.net/gh/yaling888/%s@release/%s",
			"https://testingcf.jsdelivr.net/gh/yaling888/%s@release/%s",
			"https://fastly.jsdelivr.net/gh/yaling888/%s@release/%s",
		}
	)
	for _, m := range mirrors {
		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf(m, name, file), nil)
		if err != nil {
			continue
		}

		log.Info().Msgf("[Config] try to download %s from %s", file, req.Host)

		convert.SetUserAgent(req.Header)

		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				continue
			}
			return
		}
	}
	return
}
