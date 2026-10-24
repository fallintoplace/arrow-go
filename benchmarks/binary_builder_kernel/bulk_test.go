package bulk

import (
 "bytes"
 "encoding/binary"
 "fmt"
 "math/bits"
 "strings"
 "testing"
 "unsafe"
)

// This standalone benchmark deliberately isolates Arrow Go's before/after
// offset/payload append loops. It is NOT an Arrow package end-to-end benchmark.
type benchState struct {
  offsets []byte
  values []byte
  bitmap []byte
  offLen, valLen, rowLen, nulls, offsetWidth int
  appendOffset func(int)
}

func newState(n, dataWidth, offsetWidth int) *benchState {
  s:=&benchState{offsets:make([]byte,(n+1)*offsetWidth), values:make([]byte,n*dataWidth), bitmap:make([]byte,(n+7)/8),offsetWidth:offsetWidth}
  if offsetWidth==4 {
    s.appendOffset=func(n int) { binary.LittleEndian.PutUint32(s.offsets[s.offLen:],uint32(n)); s.offLen+=4 }
  }else{
    s.appendOffset=func(n int) { binary.LittleEndian.PutUint64(s.offsets[s.offLen:],uint64(n)); s.offLen+=8 }
  }
  return s
}
func (s *benchState) reset() {s.offLen=0; s.valLen=0; s.rowLen=0; s.nulls=0}
func(s *benchState) appendNextOffset(){s.appendOffset(s.valLen)}
func(s *benchState) unsafeAppend(v []byte){copy(s.values[s.valLen:],v);s.valLen+=len(v)}
func pack8(v []bool) byte {
 var n byte
 if v[0] {n|=1<<0};if v[1] {n|=1<<1};if v[2] {n|=1<<2};if v[3] {n|=1<<3};if v[4] {n|=1<<4};if v[5] {n|=1<<5};if v[6] {n|=1<<6};if v[7] {n|=1<<7};return n
}
func(s *benchState) appendBitmap(valid []bool, n int) {
 if len(valid)==0 {
   for i:=0;i<(n+7)/8;i++{s.bitmap[i]=0xff}
 }else{
   k:=0
   for ;k+8<=n;k+=8 {v:=pack8(valid[k:]);s.bitmap[k/8]=v;s.nulls+=8-bits.OnesCount8(v)}
   if k<n {var v byte; for j:=k;j<n;j++{if valid[j]{v|=1<<uint(j-k)}else{s.nulls++}};s.bitmap[k/8]=v}
 }
 s.rowLen+=n
}
func oldAppend[T ~string|~[]byte](s *benchState,values []T,valid []bool){
 for _,v:=range values {s.appendNextOffset();s.unsafeAppend([]byte(v))}
 s.appendBitmap(valid,len(values))
}
func directAppend[T ~string|~[]byte](s *benchState,values []T,valid []bool){
 pos:=s.valLen
 if s.offsetWidth==4{
   dest:=unsafe.Slice((*int32)(unsafe.Pointer(&s.offsets[s.offLen])),len(values))
   for i,v:=range values {dest[i]=int32(pos);pos+=copy(s.values[pos:],v)}
 }else{
   dest:=unsafe.Slice((*int64)(unsafe.Pointer(&s.offsets[s.offLen])),len(values))
   for i,v:=range values {dest[i]=int64(pos);pos+=copy(s.values[pos:],v)}
 }
 s.offLen+=len(values)*s.offsetWidth
 s.valLen=pos
 s.appendBitmap(valid,len(values))
}
var sink int
func benchTyped[T ~string|~[]byte](b *testing.B,values []T,valid []bool,offsetWidth int, old bool) {
  rows:=len(values)
  dataLen:=0
  for _,v:=range values {dataLen+=len(v)}
  s:=newState(rows,(dataLen+rows-1)/rows,offsetWidth)
  b.SetBytes(int64(dataLen));b.ReportAllocs();b.ResetTimer()
  for i:=0;i<b.N;i++{s.reset();if old{oldAppend(s,values,valid)}else{directAppend(s,values,valid)}}
  sink=s.valLen+s.offLen+s.nulls+s.rowLen
}
func BenchmarkAppendKernel(b *testing.B) {
 for _,kind:=range []string{"binary","large_binary","string","large_string"} {
   offsetWidth:=4; if strings.HasPrefix(kind,"large_"){offsetWidth=8}
   isString:=strings.HasSuffix(kind,"string")
   for _,rows:=range []int{1024,65536}{
    for _,width:=range []int{4,16,64,256}{
     vbytes:=make([][]byte,rows);vstrings:=make([]string,rows)
     payload:=bytes.Repeat([]byte("a"),width)
     for i:=range vbytes{vbytes[i]=payload;vstrings[i]=string(payload)}
     for _,pat:=range []string{"all_valid","10pct_null","50pct_null"}{
       var valid []bool
       if pat!="all_valid" {valid=make([]bool,rows);for i:=range valid{if pat=="10pct_null"{valid[i]=i%10!=0}else{valid[i]=i%2!=0}}}
       for _,impl:=range []string{"old","direct"}{
         b.Run(fmt.Sprintf("%s/%s/rows_%d/width_%d/%s",impl,kind,rows,width,pat),func(b *testing.B){
          if isString{benchTyped(b,vstrings,valid,offsetWidth,impl=="old")}else{benchTyped(b,vbytes,valid,offsetWidth,impl=="old")}
         })
       }
     }
    }
   }
 }
}
func TestParity(t *testing.T){
 for _,offsetWidth:=range []int{4,8}{for _,rows:=range []int{1,8,9,15,64,1024}{
  values:=make([]string,rows);valid:=make([]bool,rows)
  for i:=range values{values[i]=strings.Repeat("x",i%51);valid[i]=i%7!=0}
  x:=newState(rows,50,offsetWidth);y:=newState(rows,50,offsetWidth)
  oldAppend(x,values,valid);directAppend(y,values,valid)
  if x.offLen!=y.offLen||x.valLen!=y.valLen||x.nulls!=y.nulls||!bytes.Equal(x.offsets,y.offsets)||!bytes.Equal(x.values,y.values)||!bytes.Equal(x.bitmap,y.bitmap){t.Fatalf("mismatch offsetWidth=%d rows=%d",offsetWidth,rows)}
 }}
}
