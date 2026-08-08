# IIG StageX — Mobile App Design (ClientDesignMobile)

**Product:** IIG StageX — Participant Mobile App (Android & iOS)
**Document type:** Detailed design for review & build
**Companion artifact:** `stagex-mobile-app-wireframes.html` — interactive phone simulator, 11 screens
**Inherits:** `ClientDesignWeb.md` v1.0 — same family-account model, APIs and business rules
**Version:** 1.1 · July 2026 *(v1.1 — R1 fields: DOB/gender/Aadhaar mandatory; school/city/guru optional)*

---

## 1. How to review

Open `stagex-mobile-app-wireframes.html` in a browser. It renders a **realistic phone simulator** — status bar, dynamic island, in-app bottom tab bar — that you can tap through exactly like the shipped app. The panel beside the phone jumps to any screen for demos. Interactive behaviors: OTP auto-advance, filter bottom sheet, live price updates on participant selection, coupon re-pricing, **payment retry counter (3 attempts) with lockout**, winners reveal, star ratings, chat quick-replies, and **certificate download with progress → "saved to phone · available offline."**

### 1.1 Screen map

| # | Screen | Covers |
|---|---|---|
| ① | Login / OTP / Password (`m-auth`) | Phone+OTP registration, password setup, dual login, biometric hint |
| ② | Home (`m-home`) | Greeting, search pill, category + filter chips, **filter bottom sheet**, event cards |
| ③ | Event detail (`m-event`) | Hero, rounds, categories, slots, sticky Register bar |
| ④ | Participants (`m-register`) | Multi-select family, eligibility ticks, 20-second ad-hoc add, sticky price bar |
| ⑤ | Payment (`m-pay`) | Order lines, coupon, **UPI apps row / QR / card / bank transfer (Razorpay)**, fail ×3 |
| ⑥ | Confirmation (`m-done`) | Entry IDs, share row (Email/SMS/WhatsApp/link/PDF), offline entry QRs |
| ⑦ | My Events (`m-events`) | Per-participant event photo + note, winners panel, feedback/cert shortcuts |
| ⑧ | My People (`m-people`) | Family list, optional profile edit (photo/video/DOB/bio) |
| ⑨ | Certificates (`m-certs`) | **Offline vault**, download-to-phone with progress, open offline, empty state |
| ⑩ | Feedback (`m-feedback`) | Star ratings (judges, venue, food stalls, schedule, sponsors) + comment |
| ⑪ | Assistant (`m-assist`) | Chat + 🎙 voice registration, quick-reply chips, safe handoff to payment |

**Navigation model:** 5-item bottom tab bar — Home · Events · **✦ Assistant (raised center button)** · Certs · People — with contextual sticky action bars (Register →, Pay now →) floating above it during flows.

---

## 2. Parity with web — confirmed coverage

Every web flow (ClientDesignWeb §§3–10) exists in the app with identical business rules:

- **One phone = family account**; OTP registration with resend timer, 3-attempt limit, password setup; login via password *or* OTP (§3 web).
- **Search & filters** — name, location, category, **age band, fees, rounds**, mode, date — presented mobile-natively as a **bottom-sheet filter panel** with chip toggles.
- **Minimal-info event cards** → event detail → **3-step registration** with multi-participant selection, per-person eligibility check (age band derived from DOB), and the **add-person form** — mandatory: name, **DOB, gender, Aadhaar number** (text, masked after save, encrypted); optional: **school, city, guru name** — collectable at registration **or later from My People** (review comment R1).
- **Payment via Razorpay** with a mobile-first method order: **UPI apps intent row (GPay / PhonePe / Paytm / BHIM) first**, then QR, card, bank transfer, wallets. Coupon apply re-prices. **Failure → retry up to 3 attempts** with visible counter, 30-minute entry hold, lockout state after the third attempt.
- **Confirmation** with per-participant entry IDs and share options (Email, SMS, WhatsApp, copy link, receipt PDF saved to phone).
- **My Events** — per-participant **event-scoped photo** (rule shown on screen: *this event only*; global photo is only a fallback) and **note per participant per event**; completed events expose **🏆 winners** and link to old-event results.
- **My People** — optional progressive profiles (photo, intro video, DOB, bio) clearly separated from event-scoped photos.
- **Certificates** — full family list with event, position, participant name, cert ID; **download per certificate** (see §3, the mobile-specific upgrade).
- **Feedback** — star questions (overall, judges & fairness, venue, **food stalls**, schedule, **sponsor booths**) + free-text + anonymous toggle.
- **Assistant** — chat and **voice (mic)** registration; finds age/fee-eligible events, registers only after explicit confirmation, and always hands off to the secure payment screen.

---

## 3. Mobile-specific design (the deltas)

### 3.1 Offline certificates — download & save to phone ⬅ new requirement
- Every certificate has a **⬇ Save** action: downloads the PDF with a visible progress bar, then flips to **"✓ Saved to phone · available offline · Open in Files."**
- Storage is dual:
  1. **Device file system** — `Files/Downloads → StageX/` on Android (MediaStore/SAF), Files app → On My iPhone → StageX on iOS — so the PDF survives app uninstall and can be printed/shared from anywhere.
  2. **In-app offline vault** — an encrypted app-storage copy listed under the "📴 Offline vault" banner (count + total MB), openable with zero connectivity (airplane-mode safe), e.g., showing a certificate at a school office with no signal.
- Rules: re-download replaces the copy if the certificate is ever re-issued (versioned by cert ID); the **verification QR is embedded in the PDF** so authenticity checks work from the offline copy; "Offline only" filter chip lists what's on the device; delete-from-device frees space without affecting the cloud copy.
- Entry QR codes get the same treatment automatically at payment success ("Entry QRs added to your phone — work offline at the gate").

### 3.2 Native capabilities used
- **OTP auto-read** (Android SMS Retriever / iOS autofill) — the wireframe notes "auto-reads SMS."
- **Biometric login** (Face ID / fingerprint) after first password/OTP login.
- **UPI intent deep-links** — tapping GPay/PhonePe opens the app directly with the Razorpay order; return handled via deep link.
- **Push notifications** — the app channel for all event triggers (registration confirmed, schedule change, round reminder, results out) configured per event by the Event Admin (Admin Design §5.5); notification tap deep-links to the exact screen.
- **Voice input** — on-device speech-to-text for the Assistant mic.
- **Camera/gallery** pickers for global and event-scoped photos; client-side compression before upload.

### 3.3 Mobile UX conventions
- Bottom sheets for filters and secondary actions; sticky bottom action bars during flows; pull-to-refresh on lists; skeleton loaders; haptic feedback on success moments (payment, certificate saved); safe-area padding; dark-status-bar treatment over hero imagery.
- Touch targets ≥ 44 px; one-hand reach: all primary CTAs in the bottom third.
- **Low-bandwidth mode:** image quality degrades gracefully; payment (QR renders locally) and the offline vault work with no/poor connectivity.

### 3.4 Tech notes (for the build discussion)
- Recommended: a single cross-platform codebase (React Native or Flutter) against the **same APIs as web** (ClientDesignWeb §11.1) — no new backend surface except `GET /certificates/:id/file` with resumable download headers and a `POST /devices` push-token registration.
- Offline vault: SQLite/secure storage index + file store; certificate files cached with checksum; background re-sync when online.
- Analytics events mirror web with `platform=app`; assistant registrations tagged `source=assistant`.

---

## 4. Business rules recap (unchanged from web, restated for the app team)

1. One phone number = one family account; people reusable across events.
2. Person record: **mandatory** = name, DOB, gender, Aadhaar (text, masked, encrypted); **optional** = school, city, guru — at registration or later from My People; age band derives from DOB.
3. **Event photos are entry-scoped** — never shown in other events; global photo is only fallback.
4. Eligibility (age band × category) validated before payment.
5. Coupons from the Ops pool; one per order (v1).
6. Payment: entries held 30 min; **max 3 retries**; idempotent webhooks; auto-refund on capture-after-fail.
7. Confirmation share: Email / SMS / WhatsApp / link / PDF.
8. Notes are per participant per event; feedback per family per event, post-completion, anonymous default.
9. Winners of past events permanently viewable; certificates QR-verifiable — **and on mobile, downloadable to the device for offline use (§3.1)**.
10. Assistant requires explicit confirmation; payment never happens in chat.

---

## 5. Open questions

- Android + iOS at launch, or Android-first (likely majority of the audience)?
- Should the offline vault be capped (e.g., 50 MB) with LRU eviction, or unlimited?
- Offline entry QRs: refresh window before event day (24 h?) to prevent stale QRs.
- Regional-language app UI at launch (Hindi + 2?) — affects store listings too.
- App-only incentives (e.g., push-based "slots almost full" nudges) — marketing to confirm.

## 6. Demo script (2½ minutes)

1. **①** Phone number → OTP boxes → set password → land on Home. (25 s)
2. **②** Tap the search pill — the **filter bottom sheet** slides up (fees, rounds, age, mode) → apply. (20 s)
3. **③→④** Open Nritya Mahotsav → Register → toggle Diya on (sticky bar re-prices) → show the ad-hoc add card. (30 s)
4. **⑤** Apply EARLYBIRD20 (total drops) → point at the UPI apps row + QR → tap **Pay now** three times to walk the retry counter into lockout → "Demo: success." (35 s)
5. **⑥** Share chips → My Events: save Ananya's note, note the event-only photo rule → open **winners**. (25 s)
6. **⑨** Certificates: tap **⬇ Save** on Kabir's Gold — progress bar → *"saved to phone, available offline"*, vault counter updates; open the already-offline Silver with no internet. (30 s)
7. **⑩→⑪** Star the food stalls & sponsors, submit → Assistant: voice mic + chat registration with safe payment handoff. (25 s)

---

*End of ClientDesignMobile v1.0 — same account model, same rules, native ergonomics, and certificates that live on the phone.*
