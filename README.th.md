# AKEF SKPort Claim

[English](README.md)

`akef-claim` เป็นเครื่องมือ command line แบบไม่เป็นทางการสำหรับตรวจและรับรางวัลเช็กอินรายวัน SKPORT ของ Arknights: Endfield ทำงานภายในเครื่องผู้ใช้เท่านั้น โครงการนี้ไม่เกี่ยวข้องกับ Hypergryph หรือ Gryphline และการทำงานอัตโนมัติอาจมีความเสี่ยงต่อบัญชี โปรดใช้อย่างระมัดระวัง

โปรแกรมไม่มี server, cloud claim, captcha/anti-bot bypass, browser automation, fingerprint spoofing หรือ proxy rotation

## ความปลอดภัย

ค่า `cred` และ `game_role` เป็น session secret ควรป้องกัน `config.toml` เหมือนรหัสผ่าน ห้ามแนบ config จริง, cookie, request header, bot token, chat ID, webhook URL หรือ log ที่ยังไม่ปกปิดลงใน issue หากข้อมูลรั่วให้ logout/หมุน session และลบข้อมูลออกจาก Git history

## ติดตั้งและตั้งค่าครั้งแรก

ดาวน์โหลดและแตก archive ให้ตรงกับระบบปฏิบัติการ แต่ละ release มี executable เพียงไฟล์เดียว:

- Windows: `akef-claim.exe`
- Linux/macOS: `akef-claim`

installer ใช้ Bash เท่านั้น บน Windows ให้เปิด Git Bash แล้วรัน:

```bash
./scripts/install.sh
```

การเรียก installer ครั้งแรกจะติดตั้ง executable, สร้าง `config.toml` และแสดงตำแหน่งไฟล์ ให้แก้ placeholder แล้วเรียก installer อีกครั้ง โดยจะยังไม่ติดตั้ง scheduler จนกว่า config จะผ่านการตรวจสอบ

หาก build จาก source ต้องใช้ Go 1.26.5:

```bash
make build
```

binary จะอยู่ใต้ `bin/` หรือ build ด้วย Go โดยตรง:

```bash
go build -trimpath ./cmd/akef-claim
```

### วิธีดูค่า header ของบัญชี

ใช้เฉพาะ session ของบัญชีตนเอง:

1. เปิดหน้าเช็กอิน Endfield SKPORT ทางการที่ `https://game.skport.com/endfield/sign-in` จากนั้นเข้าสู่ระบบและเลือก game role ที่ต้องการ
2. เปิด Developer Tools ของ browser (`F12` หรือ **Inspect**) แล้วเลือกแท็บ **Network**
3. reload หน้าแล้วกรอง request ด้วย `/web/v1/game/endfield/attendance`
4. เลือก attendance request แล้วเปิด **Headers** → **Request Headers**
5. คัดลอกเฉพาะค่าของ `Cred` ไปใส่ `accounts[].cred`
6. คัดลอกเฉพาะค่าของ `Sk-Game-Role` ไปใส่ `accounts[].game_role`

ชื่อ HTTP header ไม่แยกตัวพิมพ์เล็กและใหญ่ browser จึงอาจแสดงเป็น `cred`, `Cred`, `sk-game-role` หรือ `Sk-Game-Role` ห้ามคัดลอก `Sign` หรือ `Timestamp` เพราะโปรแกรมจะสร้างค่าเหล่านี้ใหม่ทุก request ที่ต้องลงลายเซ็น

```toml
[[accounts]]
name = "main"
enabled = true
cred = "<CRED_HEADER_VALUE>"
game_role = "<SK_GAME_ROLE_HEADER_VALUE>"
language = "en"
```

ห้ามใช้ **Copy as cURL**, export ไฟล์ HAR, แนบภาพหน้าจอ หรือแชร์ request headers ทั้งชุด เพราะมักมี credential อื่นรวมอยู่ด้วย หากค่าใดรั่วให้ logout จาก SKPORT, login ใหม่ และเปลี่ยนค่าใน config ก่อนใช้งานต่อ เมื่อตั้งค่าหลายบัญชีหรือหลาย role ให้สลับบัญชี/role แล้วทำขั้นตอนซ้ำ

### ตั้งค่าให้เสร็จ

แก้ไฟล์ TOML ที่ installer แสดง โปรแกรมอ่าน TOML เท่านั้นและไม่อ่าน environment variable เป็น fallback โครงสร้าง config ทั้งหมดอยู่ใน [docs/configuration.md](docs/configuration.md) และ [config.example.toml](config.example.toml)

หลังบันทึก config ให้รัน installer ซ้ำ script จะตรวจ config และติดตั้ง scheduler ของระบบปฏิบัติการโดยตรง ค่าเริ่มต้นคือ `00:05` ตาม timezone ของผู้ใช้ และเปลี่ยนได้ด้วย `--time HH:MM`:

```bash
./scripts/install.sh --time 00:05
```

เมื่อ directory ที่ติดตั้งอยู่ใน `PATH` ให้ตรวจตัวโปรแกรมแยกด้วย:

```bash
akef-claim config path
akef-claim config validate
akef-claim status
```

การสร้างและลบ scheduler เป็นหน้าที่ของ Bash scripts โดยตั้งใจ ไม่ใช่ CLI subcommand

หากมีหลายบัญชี ให้เพิ่ม `[[accounts]]` อีกชุดโดยใช้ `name` ที่ไม่ซ้ำ แล้วทำขั้นตอนดึง header ใหม่ขณะเข้าสู่บัญชีและ role ที่ต้องการ

## ใช้งาน

```bash
akef-claim run
akef-claim run --account main
akef-claim status
akef-claim doctor
akef-claim doctor --network
akef-claim notify test discord-home
akef-claim --silent run
```

`status` ไม่ claim ส่วน `run` จะ refresh session, ตรวจสถานะก่อน และส่ง claim POST ไม่เกินหนึ่งครั้งเมื่อมีรางวัลที่รับได้เท่านั้น ถ้าผลของ claim กำกวม โปรแกรมจะไม่ retry อัตโนมัติ หาก attendance item เดียวกันมี `available=true` และ `done=true` ขัดแย้งกัน โปรแกรมจะ fail closed และถือว่า claim แล้วเพื่อไม่ส่ง POST ซ้ำ

โปรแกรมทำ startup jitter ก่อนถือ process lock งาน `run` ที่ซ้อนกันจะรอ lock สูงสุด 10 นาทีแล้วตรวจ attendance ใหม่ จึงไม่เงียบและข้ามงานทั้งวัน ส่วน `status` เป็น read-only และไม่ถือ claim lock

## Scheduler

```bash
./scripts/install.sh --time 00:05
./scripts/uninstall.sh
./scripts/uninstall.sh --purge
```

- Windows: `install.sh` สร้างหรือแทนที่ task ด้วย `schtasks.exe /Create /TN "AKEF SKPort Daily Claim" /XML ... /F` แล้วลบ XML ชั่วคราวเมื่อเสร็จ Task จะเรียก process เดิมเป็น `akef-claim.exe --silent run` ผ่าน PowerShell ที่มากับ Windows แบบซ่อนหน้าต่าง ส่วน `uninstall.sh` ลบ task ด้วย `schtasks.exe /Delete /TN ... /F`
- Linux: installer เขียนและเปิดใช้ systemd service/timer ระดับผู้ใช้ หากไม่มี systemd user manager ที่ใช้งานได้ จะติดตั้ง crontab block ที่มี marker ของโครงการโดยไม่แก้รายการอื่น
- macOS: installer เขียนและโหลด LaunchAgent ระดับผู้ใช้

scheduled invocation แบบ silent มี deadline ภายในโปรแกรม 30 นาที ฝั่ง Windows เพิ่ม execution limit 30 นาทีใน Task Scheduler และ retry สูงสุด 3 ครั้ง ห่างกัน 30 นาที เฉพาะเมื่อโปรแกรมคืน transient pre-claim exit code `30` เท่านั้น Linux และ macOS ไม่เพิ่ม process retry และ claim POST จะไม่ถูก retry อัตโนมัติทุกระบบ

log ของ scheduled run แยกเป็นไฟล์รายวันใต้ user cache directory ของระบบ ทุกครั้งที่เริ่ม silent mode โปรแกรมจะลบเฉพาะ scheduled log ของ AKEF ที่เก่ากว่า 45 วัน และ rotate ไฟล์ของวันปัจจุบันเมื่อเกิน 5 MiB การ uninstall ปกติจะเก็บ config, log และ notification state ไว้ ต้องใช้ `--purge` จึงลบข้อมูลเหล่านี้

GitHub Actions ใช้เฉพาะ build/test CI ตอน push หรือ pull request งานเช็กอินรายวันต้องรันผ่าน scheduler ภายในเครื่องผู้ใช้ repository ไม่มี scheduled Actions workflow และไม่เก็บ credential สำหรับเช็กอินบน GitHub ดูรายละเอียดที่ [docs/scheduler.md](docs/scheduler.md)

## Notification

รองรับ Discord, Telegram และ ntfy การทดสอบเป็นข้อความสังเคราะห์และไม่เรียก SKPORT:

```bash
akef-claim notify test telegram-admin
```

notification ล้มเหลวจะไม่ทำให้เกิด claim เพิ่ม ดู [docs/notifications.md](docs/notifications.md)

## Exit code

- `0`: สำเร็จ, claim แล้ว หรือยังรับไม่ได้
- `10`: config ผิด
- `20`: session หมดอายุ
- `30`: transient failure ก่อน claim รวมถึง network/server, lock timeout หรือ scheduled deadline
- `40`: claim API ปฏิเสธแน่นอน
- `41`: ผล claim กำกวม ห้าม retry อัตโนมัติ
- `50`: internal error

scheduled silent mode ใช้ exit code ชุดเดียวกับ interactive mode โดย Windows task จะ map เฉพาะ exit code `30` ให้เป็น failure ที่ Task Scheduler retry ได้ ดังนั้น exit code `40` และ `41` จะไม่ทำให้เกิด automatic claim attempt รอบใหม่ ส่วน Linux และ macOS ไม่เพิ่ม process retry

## พัฒนา

repository มี Make targets ที่ใช้ Bash:

```bash
make repo-check
make check
make ci
make build
make install SCHEDULE_TIME=00:05
make uninstall
make snapshot
```

`make repo-check` ปฏิเสธไฟล์ที่มีลักษณะเป็น secret หรือค่าเก่าที่ไม่ควรถูก track ส่วน `make check` จะตรวจ module, tidy state, Go formatting, syntax ของ Bash, vet และ test ด้วย `make ci` เพิ่ม race detector และ build ระบบปัจจุบัน `make install` กับ `make uninstall` จะเรียก Bash scheduler scripts และ `make snapshot` ต้องติดตั้ง GoReleaser เพื่อสร้าง release archive ภายในเครื่องโดยไม่ publish

## แก้ปัญหาและรายงาน

หากมีปัญหาให้รัน `akef-claim doctor` และอ่าน [docs/troubleshooting.md](docs/troubleshooting.md) ก่อนรายงาน ห้ามเปิดเผย secret ทุกชนิด ดูการรายงานช่องโหว่ที่ [SECURITY.md](SECURITY.md)

## License

เลือกใช้ได้ภายใต้ Apache License 2.0 หรือ MIT License ดู [LICENSE-APACHE](LICENSE-APACHE) และ [LICENSE-MIT](LICENSE-MIT)
