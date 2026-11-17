# RVO

Balas gambar, video, atau voice note sekali lihat dengan `.rvo` atau
`.readviewonce`. Bot mengirim ulang medianya sebagai pesan biasa di chat yang
sama. Caption gambar/video dan sifat voice note dipertahankan. Command memakai
cooldown dan quota standar bot, serta tampil otomatis di menu.

RVO mencari pesan asli di store berdasarkan akun, chat, ID, pengirim, dan arah
pesan. Flag sekali lihat dipulihkan juga, karena isi quote dapat kehilangan flag
atau metadata download. Jika pesan asli tidak ada, RVO mencari metadata lengkap
di quote pesan lain yang sudah diterima di chat yang sama, dengan ID dan pengirim
target yang cocok. Quote dicatat sebagai pengamatan, tidak menggantikan atau
dianggap sebagai event asli pengirim. Jika store tidak menemukan pesan, quote lengkap yang
masih memiliki flag sekali lihat tetap dapat dipakai. Pesan media biasa ditolak.
Pesan sebelum store aktif, yang kedaluwarsa, atau yang tidak diterima bot dapat
tidak ditemukan. File yang sudah tidak tersedia di WhatsApp tetap bisa gagal.

WhatsApp dapat tidak mengirim payload view-once ke perangkat tertaut bot sama
sekali. Jika quote juga tidak memiliki direct path/key/hash, store tidak dapat
menciptakannya. RVO membalas "Media belum bisa diambil, yaa." ketika metadata belum tersedia,
dan `.ceksystem` menghitung event view-once yang dilaporkan tidak tersedia oleh
WhatsApp sejak proses berjalan. Membalas pesan yang sama dari perangkat lain
yang masih menyertakan metadata lengkap dapat menyediakan data untuk RVO.

Download/dekripsi dan upload/enkripsi memakai file sementara, tanpa membaca
seluruh file ke RAM atau menjalankan ffmpeg. Data hasil upload memakai media key
baru; hasil tidak membawa wrapper/flag view-once ataupun context asli pengirim.
File sementara dihapus ketika operasi selesai atau gagal.

Batas awal: 64 MiB per media, dua transfer RVO sekaligus, dan timeout 90 detik
termasuk menunggu slot. Ukuran dari metadata diperiksa sebelum download, sedangkan
jumlah byte yang benar-benar ditulis juga dibatasi walaupun metadata ukurannya
kosong/salah. Ruang untuk padding/MAC dienkripsi diperhitungkan terpisah. Tidak
ada pengunduhan media otomatis oleh store atau permintaan upload ulang media.

Tes transport menggunakan stub untuk memeriksa download/upload/send, pesan biasa
hasilnya, kegagalan tiap tahap, cleanup file, batas ukuran, dan pembatalan.
Tes integrasi store memeriksa pemulihan flag view-once dan isolasi chat. Pengiriman
melalui akun WhatsApp nyata perlu diuji setelah deploy.
