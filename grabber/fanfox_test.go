// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

// realChapterfunPayload is a real (captured) chapterfun.ashx response for
// Chainsaw Man chapter 1, page 1: a P.A.C.K.E.R.-packed script that builds
// an array of 2 image URLs (the current page, and the next page's preload).
const realChapterfunPayload = `eval(function(p,a,c,k,e,d){e=function(c){return(c<a?"":e(parseInt(c/a)))+((c=c%a)>35?String.fromCharCode(c+29):c.toString(36))};if(!''.replace(/^/,String)){while(c--)d[e(c)]=k[c]||e(c);k=[function(e){return d[e]}];e=function(){return'\w+'};c=1;};while(c--)if(k[c])p=p.replace(new RegExp('\b'+e(c)+'\b','g'),k[c]);return p;}('t e(){2 h="//7.a.3/c/6/4/5.0/b";2 1=["/k.f?g=m&8=9","/j.f?g=l&8=9"];n(2 i=0;i<1.s;i++){r(i==0){1[i]="//7.a.3/c/6/4/5.0/b"+1[i];o}1[i]=h+1[i]}p 1}2 d;d=e();q=0;',30,30,'|pvalue|var|me|29295|001|manga|zjcdn|ttl|1784822400|mangafox|compressed|store||dm5imagefun|jpg|token|pix||n000a|n000|320dccb5aaee682d6ff751ab5b3ab11d64659994|8a84017bd636f7b271b3f593fe70631923655014|for|continue|return|currentimageid|if|length|function'.split('|'),0,{}))`

func TestUnpackPackerJS(t *testing.T) {
	got, err := unpackPackerJS(realChapterfunPayload)
	if err != nil {
		t.Fatalf("unpackPackerJS() error = %v", err)
	}

	want := `function dm5imagefun(){var pix="//zjcdn.mangafox.me/store/manga/29295/001.0/compressed";var pvalue=["/n000.jpg?token=8a84017bd636f7b271b3f593fe70631923655014&ttl=1784822400","/n000a.jpg?token=320dccb5aaee682d6ff751ab5b3ab11d64659994&ttl=1784822400"];for(var i=0;i<pvalue.length;i++){if(i==0){pvalue[i]="//zjcdn.mangafox.me/store/manga/29295/001.0/compressed"+pvalue[i];continue}pvalue[i]=pix+pvalue[i]}return pvalue}var d;d=dm5imagefun();currentimageid=0;`
	if got != want {
		t.Errorf("unpackPackerJS() = %q, want %q", got, want)
	}
}

func TestUnpackFanfoxImages(t *testing.T) {
	got, err := unpackFanfoxImages(realChapterfunPayload)
	if err != nil {
		t.Fatalf("unpackFanfoxImages() error = %v", err)
	}

	want := []string{
		"//zjcdn.mangafox.me/store/manga/29295/001.0/compressed/n000.jpg?token=8a84017bd636f7b271b3f593fe70631923655014&ttl=1784822400",
		"//zjcdn.mangafox.me/store/manga/29295/001.0/compressed/n000a.jpg?token=320dccb5aaee682d6ff751ab5b3ab11d64659994&ttl=1784822400",
	}
	if len(got) != len(want) {
		t.Fatalf("unpackFanfoxImages() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unpackFanfoxImages()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestJsIntVar(t *testing.T) {
	src := `var comicid = 29295;var chapterid =568779;var imagecount=53;`

	cases := []struct {
		name string
		want int
	}{
		{"comicid", 29295},
		{"chapterid", 568779},
		{"imagecount", 53},
	}

	for _, c := range cases {
		got, err := jsIntVar(src, c.name)
		if err != nil {
			t.Errorf("jsIntVar(%q) error = %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("jsIntVar(%q) = %d, want %d", c.name, got, c.want)
		}
	}

	if _, err := jsIntVar(src, "missing"); err == nil {
		t.Error("jsIntVar(\"missing\") expected an error, got nil")
	}
}
