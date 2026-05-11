# Pre-production security checklist

Audit din 2026-05-11. MQTT password fallback rezolvat în cod
(`agent/security-mqtt-password-required`). Restul sunt items de aplicat la
deploy production — toate sunt config sau infra, nu schimbări de cod.

Severitate: 🔴 blocker pentru prod | 🟠 hardening necesar | 🟡 nice to have.

---

## 🔴 1. TLS termination

**Risc:** JWT-uri, refresh tokens, parole MQTT, payload-uri de provisioning
călătoresc în clar pe LAN. Un attacker cu acces la WiFi / port mirror /
ARP spoof interceptează totul.

**Fix:**
- Kong: activează ascultarea pe `:8443` cu cert Let's Encrypt sau cert
  intern (mTLS opțional pentru `/api/mqtt/*` hook endpoints).
- Django + Go: ascultă doar pe `127.0.0.1` (sau private network); Kong e
  singura intrare externă.
- EMQX: dezactivează `:1883`, activează `:8883` cu același cert. ESP32-urile
  cu PubSubClient + WiFiClientSecure (sau ArduinoBearSSL).
- HSTS header pe Kong response.

**Verificare:** `nmap -p 80,443,1883,8883,8000,8090 <host>` — doar 443 + 8883
ar trebui să fie open externally.

---

## 🔴 2. Go API bind pe 127.0.0.1

**Risc:** `cmd/main.go:192` bind-uiește pe `0.0.0.0:8090`. Orice host de pe
LAN poate să lovească direct, ocolind Kong (deci ocolind orice rate-limit
sau plugin viitor). Singura barieră actuală e JWT-ul re-validat de Go.

**Fix:**
```go
server := &http.Server{
    Addr:    "127.0.0.1:" + apiPort,  // în loc de 0.0.0.0
    Handler: ...,
}
```
Dacă Kong rulează pe alt VM, păstrează `0.0.0.0` dar adaugă iptables care
permite trafic doar dinspre IP-ul Kong-ului:
```bash
iptables -A INPUT -p tcp --dport 8090 -s <KONG_IP> -j ACCEPT
iptables -A INPUT -p tcp --dport 8090 -j DROP
```

---

## 🟠 3. Rate limiting

**Risc:** `/api/token/` și `/api/provisioning/activate/` sunt publice fără
limit. Brute-force pe parole user și replay pe activation tokens.

**Fix Kong plugin:**
```yaml
- name: django-auth
  routes:
    - name: django-token
      plugins:
        - name: rate-limiting
          config:
            minute: 5
            policy: local
- name: django-public
  routes:
    - name: django-provisioning
      plugins:
        - name: rate-limiting
          config:
            hour: 10
            policy: local
```

---

## 🟠 4. HS256 → RS256 (asymmetric JWT)

**Risc:** Cu HS256 oricine știe `JWT_SECRET` poate **mint tokens**.
Compromise pe Go VM = poți crea token-uri ca admin pentru orice tenant.

**Fix:** Django mintează cu private key (RS256), Kong + Go validează cu
public key. Compromise pe Go/Kong = pierzi doar verificarea, nu forjarea.

```python
# Django settings.py
SIMPLE_JWT = {
    "ALGORITHM": "RS256",
    "SIGNING_KEY": open("/etc/jwt/private.pem").read(),
    "VERIFYING_KEY": open("/etc/jwt/public.pem").read(),
}
```
Kong jwt_secrets folosește `algorithm: RS256` cu `rsa_public_key`.
Go: schimbi în `getTokenContext()` din `[]byte(secret)` în `parsePublicKey()`.

---

## ✅ 5. Service tokens (`is_service: true`) — REZOLVAT

**Status:** rezolvat în `agent/security-jwt-no-service-flag`.

Claim-ul `is_service` nu mai trăiește în JWT — middleware-ul îl derivă
server-side din `user.is_superuser` la fiecare request, cu cache TTL 60s
și invalidare prin signal pe save User.

Beneficii:
- XSS care exfiltrează JWT-ul din localStorage primește doar identitatea
  user-ului, NU bit-ul de admin. Tokenul forjat cu `is_service: true` în
  payload e ignorat (test `test_forging_is_service_claim_does_not_grant_privilege`).
- Demote admin în UI → cache invalidat instant → request-ul următor 403.

**Hardening rămas pentru sesiuni browser în general:**
- Reduce TTL: access 15 min (în loc de 1h), refresh 1 zi (în loc de 7).
- Refresh tokens în HttpOnly + SameSite=Strict cookie, nu localStorage.

---

## 🟠 6. Secrets în plain `.env`

**Risc:** `JWT_SECRET`, `DJANGO_SECRET_KEY`, `MQTT_HOOK_SECRET`, parole DB
plaintext pe disk. Backup leak → totul cade.

**Fix:**
- Minimum: `chmod 600 .env` + `chown root:root` + `chattr +i .env` (immutable).
- Mediu: SOPS cu age key, decrypt la deploy.
- Ideal: HashiCorp Vault / AWS Secrets Manager, app fetch-uiește la start.

---

## 🟠 7. Activation tokens fără binding

**Risc:** Token din `/api/provisioning/activate/` poate fi folosit din orice
IP. Token leak în log → atacator pe LAN activează el primul, setând parola
lui.

**Fix opțiuni:**
- Bind la MAC address sau CN din client cert TLS.
- IP allowlist setat la generare (`--allowed-ip 192.168.1.42`).
- Sau pur și simplu TTL foarte scurt (5 min) + token din 64 chars.

---

## 🟡 8. Kong access log filter

**Risc:** Authorization header logged → JWT-urile ajung în loguri lung-păstrate.

**Fix:** Plugin `request-transformer` care strip-uiește `Authorization`
înainte de logging, sau redact în Kong log format.

---

## 🟡 9. Capability denormalization staleness

`Device.capabilities` JSONField e populat la save() prin signals. Dacă YAML-ul
DD se schimbă, capability-urile vechi rămân în DB pe device-urile existente.

**Fix:** `python manage.py sync_capabilities` ca cron job zilnic, sau hook
on DD reload.

---

## Verificare finală pre-prod

```bash
# 1. Verifică niciun device fără hash
python manage.py force_rotate_mqtt_credentials --dry-run
# Expect: "Device-uri afectate: 0"

# 2. Verifică TLS
nmap --script ssl-cert -p 443,8883 prod.example.com
# Expect: cert valid + chain complet

# 3. Verifică porturi externe
nmap -p 1-65535 prod.example.com
# Expect: doar 443, 8883 open

# 4. Verifică env loaded corect pe Kong
docker exec kong env | grep KONG_JWT_SECRET
# Expect: secret hex, nu gol

# 5. Smoke test
TOKEN=$(curl -sk -X POST https://prod.example.com/api/token/ \
        -d 'username=test&password=...' | jq -r .access)
curl -sk -H "Authorization: Bearer $TOKEN" https://prod.example.com/go/runtime
# Expect: 200 + JSON (lista runtime)
```

---

## Status implementat

| Item | Status | Branch |
|------|--------|--------|
| MQTT password obligatoriu | ✅ done | agent/security-mqtt-password-required |
| Force rotate management cmd | ✅ done | agent/security-mqtt-password-required |
| `is_service` scos din JWT | ✅ done | agent/security-jwt-no-service-flag |
| TLS termination | ⏳ deploy | — |
| Go bind 127.0.0.1 | ⏳ deploy | — |
| Rate limiting | ⏳ deploy | — |
| RS256 migration | ⏳ pre-prod | — |
| Access/refresh TTL hardening | ⏳ pre-prod | — |
| Secrets manager | ⏳ deploy | — |
| Activation IP binding | ⏳ pre-prod | — |
