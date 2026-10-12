package fixture

// ruleid: no-io-ioutil
import "io/ioutil"

// ok: no-io-ioutil
import "os"

func read(path string) ([]byte, error) {
	// ruleid: no-io-ioutil
	_, _ = ioutil.ReadFile(path)
	// ok: no-io-ioutil
	return os.ReadFile(path)
}
