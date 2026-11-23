package supabase_storage

import ("strings";"testing")

func TestPrivateStorageConfig(t *testing.T){
 for _,tc:=range []struct{url string;good bool}{
  {"https://abc.supabase.co",true},
  {"https://abc.storage.supabase.co",true},
  {"https://attacker.example",false},
  {"http://abc.supabase.co",false},
  {"https://abc.supabase.co/path",false},
  {"https://abc.supabase.co.evil.com",false},
 }{
   _,err:=New(tc.url,"jwt-test","evolution-media")
   if (err==nil)!=tc.good{t.Errorf("url %q unexpected error %v",tc.url,err)}
 }
}

func TestObjectPathSanitization(t *testing.T){
 for _,bad:=range []string{"../secret","/root","a//b","a/../b","","a\\b"}{
  if _,err:=objectPath(bad);err==nil{t.Errorf("accepted path %q",bad)}
 }
 p,err:=objectPath("evolution-go-medias/folder/a b.png")
 if err!=nil||!strings.Contains(p,"a%20b.png"){t.Fatalf("%q %v",p,err)}
}
