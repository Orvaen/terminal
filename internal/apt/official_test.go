package apt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

// fakeNodejsOrg simule nodejs.org avec les versions de la capture d'écran du site.
func fakeNodejsOrg(t *testing.T, installed string, corrupt bool) (msiCalls *[]string) {
	t.Helper()
	key := nodeMSIKey()
	msiName := fmt.Sprintf("node-v24.22.0-%s.msi", nodeMSIArch())
	msiBody := []byte("faux installeur node 24.22.0")
	sum := sha256.Sum256(msiBody)
	if corrupt {
		sum[0] ^= 0xff
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[
 {"version":"v26.12.0","lts":false,"files":["%[1]s"]},
 {"version":"v25.9.0","lts":false,"files":["%[1]s"]},
 {"version":"v24.22.0","lts":"Krypton","files":["%[1]s"]},
 {"version":"v24.21.0","lts":"Krypton","files":["%[1]s"]},
 {"version":"v24.20.0","lts":"Krypton","files":["%[1]s"]},
 {"version":"v23.11.1","lts":false,"files":["%[1]s"]},
 {"version":"v22.23.3","lts":"Jod","files":["%[1]s"]}
]`, key)
	})
	mux.HandleFunc("/v24.22.0/SHASUMS256.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  node-v24.22.0-x86.msi\n%s  %s\n", hex.EncodeToString(make([]byte, 32)), hex.EncodeToString(sum[:]), msiName)
	})
	mux.HandleFunc("/v24.22.0/"+msiName, func(w http.ResponseWriter, r *http.Request) { w.Write(msiBody) })
	srv := httptest.NewServer(mux)

	var calls []string
	oldE, oldD, oldN, oldM := officialEnabled, nodeDist, nodeInstalled, runMSI
	officialEnabled, nodeDist = true, srv.URL
	nodeInstalled = func() (string, bool) { return installed, installed != "" }
	runMSI = func(path string) int {
		b, _ := os.ReadFile(path)
		calls = append(calls, string(b))
		return 0
	}
	t.Cleanup(func() {
		srv.Close()
		officialEnabled, nodeDist, nodeInstalled, runMSI = oldE, oldD, oldN, oldM
	})
	return &calls
}

func TestCheckNodeFindsNewerPatchAndMajors(t *testing.T) {
	fakeNodejsOrg(t, "v24.20.0", false)
	plan, ok, err := checkNode()
	if !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if plan.target.String() != "v24.22.0" {
		t.Errorf("cible %s, attendu v24.22.0 (même majeure)", plan.target)
	}
	if plan.newCurrent.String() != "v26.12.0" {
		t.Errorf("Current %s, attendu v26.12.0", plan.newCurrent)
	}
	if plan.newMajorLTS != (version{}) {
		t.Errorf("aucune LTS plus récente que la 24 n'existe, obtenu %s", plan.newMajorLTS)
	}
}

func TestCheckNodeOlderLTSLine(t *testing.T) {
	fakeNodejsOrg(t, "v22.23.3", false)
	plan, _, _ := checkNode()
	if plan.target != (version{}) {
		t.Errorf("22.23.3 est la dernière 22.x, obtenu cible %s", plan.target)
	}
	if plan.newMajorLTS.String() != "v24.22.0" {
		t.Errorf("LTS plus récente %s, attendu v24.22.0", plan.newMajorLTS)
	}
}

func TestUpgradeNodeInstallsVerifiedMSI(t *testing.T) {
	calls := fakeNodejsOrg(t, "v24.20.0", false)
	code, handled := upgradeNode(false)
	if !handled || code != 0 {
		t.Fatalf("code %d handled %v", code, handled)
	}
	if want := []string{"faux installeur node 24.22.0"}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("installeur lancé avec %q", *calls)
	}
}

func TestUpgradeNodeRejectsBadChecksum(t *testing.T) {
	calls := fakeNodejsOrg(t, "v24.20.0", true)
	code, handled := upgradeNode(false)
	if !handled || code == 0 {
		t.Fatalf("une somme de contrôle invalide doit échouer (code %d)", code)
	}
	if len(*calls) != 0 {
		t.Fatal("l'installeur ne doit pas être lancé")
	}
}

func TestUpgradeNodeAlreadyCurrent(t *testing.T) {
	calls := fakeNodejsOrg(t, "v24.22.0", false)
	if code, handled := upgradeNode(true); !handled || code != 0 || len(*calls) != 0 {
		t.Fatalf("rien à faire attendu : code %d, appels %v", code, *calls)
	}
}

func TestAptUpgradeUsesOfficialNode(t *testing.T) {
	wingetCalls := fakeWinget(t, nil)
	msi := fakeNodejsOrg(t, "v24.20.0", false)
	oldD := dockerInstalled
	dockerInstalled = func() bool { return false }
	t.Cleanup(func() { dockerInstalled = oldD })

	Run([]string{"upgrade"})
	Run([]string{"upgrade", "node"})

	if len(*msi) != 2 {
		t.Fatalf("node aurait dû être installé depuis nodejs.org deux fois, obtenu %d", len(*msi))
	}
	want := [][]string{{"upgrade", "--all", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*wingetCalls, want) {
		t.Fatalf("appels winget %v, attendu %v", *wingetCalls, want)
	}
}

func TestAptUpgradeNodeFallsBackToWinget(t *testing.T) {
	wingetCalls := fakeWinget(t, nil)
	fakeNodejsOrg(t, "", false) // node absent
	Run([]string{"upgrade", "node"})
	want := [][]string{{"upgrade", "--id", "OpenJS.NodeJS.LTS", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*wingetCalls, want) {
		t.Fatalf("appels winget %v, attendu %v", *wingetCalls, want)
	}
}
