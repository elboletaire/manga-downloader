// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

// real chapterfun.ashx responses captured from mangahere.cc (Kengan Omega,
// chapter 363, page 1 and its last page 17) - both Dean Edwards packed blobs.
const (
	mangahereSampleFirstPage = `eval(function(p,a,c,k,e,d){e=function(c){return(c<a?"":e(parseInt(c/a)))+((c=c%a)>35?String.fromCharCode(c+29):c.toString(36))};if(!''.replace(/^/,String)){while(c--)d[e(c)]=k[c]||e(c);k=[function(e){return d[e]}];e=function(){return'\w+'};c=1;};while(c--)if(k[c])p=p.replace(new RegExp('\b'+e(c)+'\b','g'),k[c]);return p;}('g a(){2 c="//5.8.7/6/9/4/b.0/3";2 1=["/k.e","/l.e"];f(2 i=0;i<1.j;i++){h(i==0){1[i]="//5.8.7/6/9/4/b.0/3"+1[i];o}1[i]=c+1[i]}n 1}2 d;d=a();m=p;',26,26,'|pvalue|var|compressed|31432|zjcdn|store|org|mangahere|manga|dm5imagefun|363|pix||jpg|for|function|if||length|i001|i002|currentimageid|return|continue|44598869'.split('|'),0,{}))`
	mangahereSampleLastPage  = `eval(function(p,a,c,k,e,d){e=function(c){return(c<a?"":e(parseInt(c/a)))+((c=c%a)>35?String.fromCharCode(c+29):c.toString(36))};if(!''.replace(/^/,String)){while(c--)d[e(c)]=k[c]||e(c);k=[function(e){return d[e]}];e=function(){return'\w+'};c=1;};while(c--)if(k[c])p=p.replace(new RegExp('\b'+e(c)+'\b','g'),k[c]);return p;}('f a(){2 b="//3.8.7/6/9/4/c.0/5";2 1=["/g.k"];j(2 i=0;i<1.e;i++){h(i==0){1[i]="//3.8.7/6/9/4/c.0/5"+1[i];n}1[i]=b+1[i]}m 1}2 d;d=a();l=o;',25,25,'|pvalue|var|zjcdn|31432|compressed|store|org|mangahere|manga|dm5imagefun|pix|363||length|function|ic16|if||for|jpg|currentimageid|return|continue|44598884'.split('|'),0,{}))`
)

func TestUnpackMangahereImageURL(t *testing.T) {
	cases := []struct {
		name string
		res  string
		want string
	}{
		{"first page (two entries: current + next preload)", mangahereSampleFirstPage, "https://zjcdn.mangahere.org/store/manga/31432/363.0/compressed/i001.jpg"},
		{"last page (single entry: no next preload)", mangahereSampleLastPage, "https://zjcdn.mangahere.org/store/manga/31432/363.0/compressed/ic16.jpg"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := unpackMangahereImageURL(c.res)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("unpackMangahereImageURL() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestUnpackMangahereImageURLError(t *testing.T) {
	if _, err := unpackMangahereImageURL("not a packed response"); err == nil {
		t.Error("expected an error for a non-packed response, got nil")
	}
}

func TestMangahereTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.mangahere.cc/manga/kengan_omega/", true},
		{"http://www.mangahere.cc/manga/kengan_omega/", true},
		{"https://mangahere.cc/manga/kengan_omega/", true},
		{"https://www.mangadex.org/title/xyz", false},
	}

	for _, c := range cases {
		m := Mangahere{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
