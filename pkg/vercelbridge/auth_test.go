package vercelbridge

import (
 "bytes"
 "strings"
 "testing"
)

func TestEncryptedAPIKeyRoundTrip(t *testing.T) {
 key:=strings.Repeat("ab",32)
 a,err:=SealAPIKey("instance-secret",key)
 if err!=nil {t.Fatal(err)}
 b,err:=SealAPIKey("instance-secret",key)
 if err!=nil {t.Fatal(err)}
 if bytes.Equal(a,b){t.Fatal("ciphertexts should use fresh random nonces")}
 plain,err:=OpenAPIKey(a,key)
 if err!=nil || plain!="instance-secret"{t.Fatalf("bad roundtrip: %q %v",plain,err)}
 if bytes.Contains(a,[]byte("instance-secret")){t.Fatal("token leaked in ciphertext")}
 a[len(a)-1]^=1
 if _,err:=OpenAPIKey(a,key);err==nil{t.Fatal("tampering was accepted")}
}

func TestInvalidEncryptionKeys(t *testing.T){
 for _,k:=range []string{"", "hello", strings.Repeat("aa",31),strings.Repeat("z",64)}{
  if err:=ValidateTokenKey(k);err==nil{t.Errorf("accepted bad key length=%d",len(k))}
 }
 if _,err:=SealAPIKey("",strings.Repeat("aa",32));err==nil{t.Fatal("accepted empty apikey")}
}
