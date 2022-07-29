# LOMBOT
## Deskripsi
Project bot telegram sederhana dan suka-suka untuk mengisi waktu luang.

## Opsi-Opsi

    -t : Token API telegram (required)
    -u : Username pengelola bot (required)
    -w : Lama menunggu jawaban (menit) (default 5)
    -i : Lama mengabaikan pesan (detik) (default 60)
    -v : Mode debug (boolean) (default false)
    -st: Timeout pesan subscription terhapus (menit) (dafault 0 -> tidak terhapus)

## Cara Menjalankan Bot
```shell
go run main.go <OPTIONS>
```
## Fungsi Bot
1. Meminta captcha kepada user yang baru join.
   1. Jika salah 3 kali maka captcha baru akan di generate.
   2. Jika user diundang oleh admin, maka tidak akan di minta mengisi captcha.
2. /admin
   - Untuk ping admin-admin grup.
3. /ban (khusus admin grup)
   - Untuk ban user, dengan cara me-reply pesan dari user yang akan di ban.
4. /halo
   - Untuk say hello.
5. /id
   - Untuk menampilkan id user yang meminta.
6. /testpoll
   - Untuk menampilkan testpolling
7. Otomatis menghapus pesan user join/keluar

Jangan lupa memberi izin untuk men-delete pesan dan mem-ban user

## Penutup
Semoga Bermanfaat
