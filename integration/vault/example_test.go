package vault_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/plugins/vault"
)

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
	f := dotenv.Parse("DB_DSN=\"user=${user} apikey=${vault:secret/data/app#apikey}\"\n", dotenv.WithPlugin(p))
	v, _, err := f.GetExpanded("DB_DSN")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)
	fmt.Println(f.Render())
	// Output:
	// user=app apikey=s3cret
	// DB_DSN="user=${user} apikey=${vault:secret/data/app#apikey}"
}
