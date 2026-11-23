// WorkiDigital extension: confidential API-key delivery over the Supabase queue.
// Tokens are sealed with AES-256-GCM before being written to PostgreSQL.
package vercelbridge

import (
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "io"
)

func decodeTokenKey(keyHex string) ([]byte,error) {
 k,err:=hex.DecodeString(keyHex)
 if err!=nil || len(k)!=32 { return nil,errors.New("BRIDGE_TOKEN_KEY must contain exactly 64 hexadecimal characters") }
 return k,nil
}

func ValidateTokenKey(keyHex string) error { _,err:=decodeTokenKey(keyHex);return err }

func SealAPIKey(apiKey, keyHex string)([]byte,error){
 if apiKey=="" { return nil,errors.New("missing apikey") }
 k,err:=decodeTokenKey(keyHex);if err!=nil{return nil,err}
 b,err:=aes.NewCipher(k);if err!=nil{return nil,err}
 gcm,err:=cipher.NewGCM(b);if err!=nil{return nil,err}
 nonce:=make([]byte,gcm.NonceSize())
 if _,err:=io.ReadFull(rand.Reader,nonce);err!=nil{return nil,err}
 return gcm.Seal(nonce,nonce,[]byte(apiKey),nil),nil
}

func OpenAPIKey(sealed []byte,keyHex string)(string,error){
 k,err:=decodeTokenKey(keyHex);if err!=nil{return "",err}
 b,err:=aes.NewCipher(k);if err!=nil{return "",err}
 gcm,err:=cipher.NewGCM(b);if err!=nil{return "",err}
 if len(sealed)<gcm.NonceSize()+gcm.Overhead(){return "",errors.New("invalid encrypted token")}
 plain,err:=gcm.Open(nil,sealed[:gcm.NonceSize()],sealed[gcm.NonceSize():],nil)
 if err!=nil{return "",errors.New("unable to decrypt apikey")}
 if len(plain)==0{return "",errors.New("empty decrypted apikey")}
 return string(plain),nil
}
