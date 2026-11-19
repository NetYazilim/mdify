# mdify (Go)

Medium makalelerini Markdown'a çeviren Go CLI (goquery + html-to-markdown v2).

## Kurulum

```sh
go install github.com/NetYazilim/mdify@latest
```

Kaynaktan derlemek için:

```sh
go build -o mdify.exe .
```

## Kullanım

```sh
mdify https://medium.com/@user/some-article-123abc > yazi.md
mdify -o yazi.md https://medium.com/@user/some-article-123abc
mdify -O -d notlar https://medium.com/@user/some-article-123abc   # notlar/<baslik-slug>.md
mdify -html kayitli.html https://medium.com/@user/some-article     # tarayıcıdan kaydedilmiş sayfa
mdify https://levent.medium.com/some-article-123abc                # alt domainli Medium blogu
mdify https://<mirror>/https://medium.com/@user/some-article-123abc # mirror/okuyucu adresi (member-only yazılar)
```

| Bayrak | Açıklama |
|---|---|
| `-o` | Çıktı dosyası (varsayılan stdout) |
| `-O`, `-d` | Başlıktan dosya adı üret, `-d` klasörüne yaz |
| `-html` | URL yerine HTML dosyasından çevir (`-` = stdin) |
| `-any-host` | Artık gerekmiyor (her `https` adresi kabul ediliyor); eski komutlar bozulmasın diye duruyor |
| `-timeout`, `-retries` | İstek zaman aşımı (15s) ve deneme sayısı (3) |
| `-v` | Ayrıntılı log |

## Özellikler

- Host kısıtı yok: medium.com, `*.medium.com`, özel domainli bloglar ve mirror adresleri bayraksız kabul ediliyor (sadece `https` zorunlu). Çevrilemeyen sayfalar dönüşüm aşamasında hata verir (ör. `<article>` yoksa).
- Mirror/okuyucu sayfaları destekleniyor; domaine bağlı değil: `https://<herhangi-bir-host>/https://medium.com/...` biçimindeki adreslerde gömülü URL doğrulanır, sayfa yapısından tanınır. Kapak resmi başlığın altına taşınır, `/img/medium/{w}/{id}` ve `/img/{w}/{id}` proxy yolları `https://miro.medium.com/v2/resize:fit:1400/{id}` adresine çevrilir, `alt="None"` / `"Post cover image"` değerleri atılır (altyazı yoksa `data-caption` kullanılır), tema başına iki kez basılan (açık/koyu) kod blokları teke indirilir, avatar, meta satırı ve mirror arayüzü (nav, içindekiler) çıkarılır. Reference linki orijinal makaleyi gösterir.
- Mirror sayfasında Medium dışı göreli resim yolları (ör. `/img/<kaynak>/…`) mirror'ın kendi adresine göre mutlak yapılır; bu resimler mirror erişilebilir olduğu sürece çalışır.
- Kod bloğu dili `data-code-block-lang` yoksa `language-x` / `lang-x` sınıfından okunur.
- Kod blokları `data-code-block-lang` üzerinden dil etiketiyle (```` ```go ````) yazılıyor.
- Embed (iframe) içeren figure'lar iframe + altyazı olarak korunuyor. Lazy-load iframe'lerde (`src` boş) adres `data-src`'den alınır; `style`/`sandbox` gibi nitelikler atılır, adressiz iframe'ler çıkarılır; `width`/`height` niteliği olmayanlara `width="100%" height="315"` verilir. Adresinde `poster=` parametresi olan videoların altına, iframe göstermeyen görüntüleyiciler için tıklanabilir kapak resmi (`[![Video](poster)](video)`) eklenir.
- SSRF koruması yok (yerel CLI); sadece https ve en fazla 5 yönlendirme, 5 MiB gövde sınırı.
- Medium bot koruması devreye girerse sayfayı tarayıcıdan kaydedip `-html` ile çevirebilirsiniz.

## Yapı

```
main.go                     CLI
internal/medium/fetch.go    HTTP + retry/backoff (429 Retry-After, 5xx)
internal/medium/convert.go  DOM temizleme + Markdown dönüşümü
internal/medium/mirror.go   Mirror/okuyucu sayfalarını Medium düzenine uyarlama
```
