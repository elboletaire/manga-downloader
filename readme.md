- Given https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f
  + Index gets chapters from https://inmanga.com/chapter/getall?mangaIdentification=d9e47ba6-7dfc-401d-a21c-19326c2ea45f
    - Data in there is stored in a double-encoded json inside `data` key
  + Chapter gets pages from https://inmanga.com/chapter/chapterIndexControls?identification=03e65759-2cd9-4e62-9228-3cf80a51594e
    - Data is in HTML here
  + Chapter images have base URL https://pack-yak.intomanga.com/images/manga/MANGA-SERIES/chapter/CHAPTER/page/PAGE/UUID
    - i.e. https://pack-yak.intomanga.com/images/manga/MANGA-SERIES/chapter/CHAPTER/page/PAGE/d0df87b5-4ac7-4490-a730-1db84c76258c
