# Dev deployment on the VPS

Push ke branch `dev` akan build dan menjalankan Kotonehara di Docker pada VPS
yang menjalankan GitHub Actions self-hosted runner. Branch `main` tetap memakai
workflow deploy Heroku.

Environment disimpan langsung di VPS pada
`/opt/oohara/kotonehara-dev.env`; file ini tidak masuk GitHub. Ganti
`github-runner` dengan akun Linux yang menjalankan self-hosted runner, lalu
buat file sebagai akun itu agar workflow dapat membacanya:

```sh
RUNNER_USER=github-runner
sudo install -d -m 700 -o "$RUNNER_USER" -g "$RUNNER_USER" /opt/oohara
sudo -u "$RUNNER_USER" touch /opt/oohara/kotonehara-dev.env
sudo chmod 600 /opt/oohara/kotonehara-dev.env
sudo -u "$RUNNER_USER" nano /opt/oohara/kotonehara-dev.env
```

Salin variabel yang dibutuhkan dari `.env.sample`, lalu atur database agar
session WhatsApp tersimpan di volume Docker yang persisten:

```env
APP_ENV=dev
DB_DRIVER=sqlite
DATABASE_URL=file:/app/data/kotonehara.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)
BASEAPI_URL=http://hararest-dev:1337/
```

Isi juga `OWNER` dan kredensial layanan yang memang ingin dipakai di dev. Gunakan
akun WhatsApp khusus testing dan database dev tersendiri, supaya sesi dev tidak
mengganggu bot produksi. Data SQLite tersimpan di `/opt/oohara/kotonehara-data/`.

Workflow membuat atau menggunakan jaringan Docker `oohara-dev` dan menjalankan
compose file `compose.dev.yml`. Pada jaringan itu bot dapat mengakses Hararest
melalui `http://hararest-dev:1337/`. Hararest sendiri hanya membuka port
`127.0.0.1:1338` pada host, untuk akses dari reverse proxy jika diperlukan.

Akun runner harus bisa menjalankan Docker dan membaca file env tersebut.
