package vault_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ubgo/textdata/plugins/vault"
)

// The plugin is a base Resolver and Lookuper, so a format calls it for
// ${vault:path#field} and for bare ${NAME}. Here it is called directly; the
// integration module shows the same instance installed in dotenv and jsonc.
func Example() {
	// A stand-in for Vault serving one KV v2 secret.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"data":{"apikey":"s3cret","user":"app"},"metadata":{"version":1}}}`)
	}))
	defer srv.Close()

	p, err := vault.New(
		vault.WithAddress(srv.URL),
		vault.WithToken("example-token"),
		vault.WithLookupPrefix("secret/data/app#"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	key, err := p.Resolve("vault", "secret/data/app#apikey") // ${vault:secret/data/app#apikey}
	if err != nil {
		fmt.Println(err)
		return
	}
	user, ok, err := p.Lookup("user") // ${user}
	fmt.Println(key)
	fmt.Println(user, ok, err)
	// Output:
	// s3cret
	// app true <nil>
}
