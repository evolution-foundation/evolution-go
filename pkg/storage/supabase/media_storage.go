// WorkiDigital modification: private Supabase Storage media adapter.
// The upstream MinIO adapter attempts a public bucket policy. This one never does.
package supabase_storage

import (
  "bytes"
  "context"
  "encoding/json"
  "errors"
  "fmt"
  "io"
  "net/http"
  "net/url"
  "strings"
  "time"

  storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
)

type Storage struct {
 base string
 jwt string
 bucket string
 client *http.Client
}

var _ storage_interfaces.MediaStorage = (*Storage)(nil)

func New(baseURL, serviceRoleJWT, bucket string) (storage_interfaces.MediaStorage,error) {
  u,err:=url.Parse(strings.TrimRight(baseURL,"/"))
  if err!=nil || u.Scheme!="https" || u.Host=="" || u.User!=nil || u.RawQuery!="" || u.Fragment!="" || u.Path!="" {
    return nil, errors.New("SUPABASE_URL must be the HTTPS project origin without a path")
  }
  // Never send a service_role token to an arbitrary host.
  if !strings.HasSuffix(strings.ToLower(u.Hostname()),".supabase.co") {
    return nil,errors.New("SUPABASE_URL must use a Supabase project hostname")
  }
  if serviceRoleJWT=="" || bucket=="" || strings.ContainsAny(bucket,"/?#\\") {
    return nil,errors.New("SUPABASE_SERVICE_ROLE_KEY and a valid bucket are required")
  }
  return &Storage{base:strings.TrimRight(baseURL,"/"),jwt:serviceRoleJWT,bucket:bucket,
    client:&http.Client{Timeout:60*time.Second}},nil
}

func objectPath(name string)(string,error){
  name=strings.TrimPrefix(name,"evolution-go-medias/")
  if name=="" || strings.HasPrefix(name,"/") || len(name)>1024{return "",errors.New("invalid filename")}
  parts:=strings.Split(name,"/")
  for i,p:=range parts{
    if p=="" || p=="." || p==".." || strings.ContainsAny(p,"\\\x00\r\n") {return "",errors.New("unsafe filename")}
    parts[i]=url.PathEscape(p)
  }
  return "evolution-go-medias/"+strings.Join(parts,"/"),nil
}

func (s *Storage) request(ctx context.Context,method,path,contentType string,body io.Reader)(*http.Response,error){
  req,err:=http.NewRequestWithContext(ctx,method,s.base+"/storage/v1"+path,body)
  if err!=nil{return nil,err}
  req.Header.Set("Authorization","Bearer "+s.jwt)
  req.Header.Set("apikey",s.jwt)
  if contentType!=""{req.Header.Set("Content-Type",contentType)}
  return s.client.Do(req)
}

func responseError(resp *http.Response)error {
  defer resp.Body.Close()
  msg,_:=io.ReadAll(io.LimitReader(resp.Body,2048))
  return fmt.Errorf("supabase storage HTTP %d: %s",resp.StatusCode,string(msg))
}
func (s *Storage) Store(ctx context.Context,data []byte,fileName,contentType string)(string,error){
  key,err:=objectPath(fileName);if err!=nil{return "",err}
  if contentType=="" {contentType="application/octet-stream"}
  dest:="/object/"+url.PathEscape(s.bucket)+"/"+key
  res,err:=s.request(ctx,http.MethodPost,dest,contentType,bytes.NewReader(data))
  if err!=nil{return "",err}
  if res.StatusCode<200 || res.StatusCode>=300{return "",responseError(res)}
  res.Body.Close()
  return s.GetURL(ctx,fileName)
}

func (s *Storage) Delete(ctx context.Context,fileName string)error{
  key,err:=objectPath(fileName);if err!=nil{return err}
  res,err:=s.request(ctx,http.MethodDelete,"/object/"+url.PathEscape(s.bucket)+"/"+key,"",nil)
  if err!=nil{return err}
  if res.StatusCode==http.StatusNotFound{res.Body.Close();return nil}
  if res.StatusCode<200 || res.StatusCode>=300{return responseError(res)}
  return res.Body.Close()
}

func (s *Storage) GetURL(ctx context.Context,fileName string)(string,error){
  key,err:=objectPath(fileName);if err!=nil{return "",err}
  payload:=bytes.NewBufferString(`{"expiresIn":604800}`)
  res,err:=s.request(ctx,http.MethodPost,"/object/sign/"+url.PathEscape(s.bucket)+"/"+key,"application/json",payload)
  if err!=nil{return "",err}
  if res.StatusCode<200 || res.StatusCode>=300{return "",responseError(res)}
  defer res.Body.Close()
  var reply struct{SignedURL string `json:"signedURL"`; SignedUrl string `json:"signedUrl"`}
  if err:=json.NewDecoder(io.LimitReader(res.Body,8192)).Decode(&reply);err!=nil{return "",err}
  relative:=reply.SignedURL
  if relative==""{relative=reply.SignedUrl}
  if relative==""{return "",errors.New("Supabase returned no signedURL")}
  u,err:=url.Parse(relative);if err!=nil{return "",err}
  if u.IsAbs(){
    origin,_:=url.Parse(s.base)
    if u.Scheme!="https" || u.Host!=origin.Host {return "",errors.New("signed URL has an unexpected host")}
    return u.String(),nil
  }
  if !strings.HasPrefix(u.Path,"/object/sign/") || u.Host!="" || u.User!=nil {
    return "",errors.New("invalid relative signed URL")
  }
  return s.base+"/storage/v1"+u.String(),nil
}
