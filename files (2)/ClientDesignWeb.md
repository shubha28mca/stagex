# IIG StageX — Participant Web Experience Design (ClientDesignWeb)

**Product:** IIG StageX — Participant-facing Web Application
**Document type:** Detailed design for review & build (Web only)
**Companion artifact:** `stagex-participant-web-wireframes.html` — interactive wireframes, 8 screens, responsive down to mobile
**Version:** 1.1 · July 2026 *(v1.1 — review comment R1: DOB, gender & Aadhaar mandatory; school, city, guru optional with update-later path)*
**Scope note:** This document covers the **web application only**. The native **Mobile App** will be specified in a separate document (ClientDesignMobile) against the same APIs and family-account model; the web wireframe is already mobile-responsive (bottom tab bar under 900 px) so it doubles as a preview of mobile ergonomics.

---

## 1. How to review

- Open `stagex-participant-web-wireframes.html` in a browser. Every flow in this document is clickable there.
- Tap the **SR avatar** (top right) to view the Login/Registration screens; use the top tabs (or bottom bar on mobile) for the rest.
- Interactive behaviors included: OTP auto-advance, participant selection updating the price, coupon apply updating totals, **payment failure with a 3-attempt retry counter**, share-confirmation chips, winners reveal, star ratings, and the chat assistant.

### 1.1 Screen map

| # | Screen (wireframe id) | Purpose |
|---|---|---|
| A | Login / Register (`s-auth`) | Phone + OTP registration, password setup, dual login |
| B | Discover (`s-discover`) | Search & filter events, minimal-info cards |
| C | Register flow (`s-register`) | 3-step: participants → payment → confirmation |
| D | My Events (`s-myevents`) | Registered events, per-event participant management, winners |
| E | My People (`s-people`) | Family members & optional global profiles |
| F | Certificates (`s-certs`) | Wins list + downloads |
| G | Feedback (`s-feedback`) | Rated questions + comment |
| H | Assistant (`s-assist`) | Chat/voice registration agent |

---

## 2. Account model — one phone, many people

The foundational concept: a **Family Account** keyed to one verified mobile number.

- The **account holder** (e.g., a parent) registers once with their phone; they can then add any number of **people** (children, self, niece, nephew, students…), each with an independent profile.
- People are reusable across all events — added once, selectable forever.
- Profiles are **optional and progressive**: a person can be created with just *name + age + relationship* and enriched later (photo, video, DOB, bio). Profile-completeness % nudges enrichment without blocking registration.
- **Two photo scopes** (a key rule throughout):
  - **Global profile photo/video** — lives in My People, used as the default everywhere.
  - **Event-scoped photo** — set inside a specific event in My Events; used *only* for that event (stage screens, judge sheets, certificates for that event) and **never shown in other events**. Fallback order: event photo → global photo → initials avatar.

---

## 3. Screen A — Registration & Login

### 3.1 New account (phone + OTP + password)
1. **Enter mobile number** → validation: 10-digit Indian mobile (live error state shown in wireframe); duplicate-number check ("this number already has an account — login instead?").
2. **OTP verification** — 6 digit boxes with auto-advance; resend timer (30 s), OTP validity 5 min, **3 attempts** then temporary lock (15 min) with rate-limiting per device/IP.
3. **Set password** — name* + password* + confirm; strength meter (8+ chars, number, symbol hint). Account is created; user lands on Discover.

### 3.2 Login (two ways, always)
- **Phone + password**, or **Phone + OTP** ("Login with OTP instead") — both first-class; forgot-password resets via OTP.
- Session: 30-day remember-me on web; sensitive actions (payment, profile edits of minors) re-verified by session freshness.

### 3.3 Rules
- One account per mobile number; number change is a support flow with re-verification.
- Minors never have their own login on web — the family account holder acts for them (per PRD SX-FR-003).

---

## 4. Screen B — Discover (search & filters)

- **Search bar:** event name (type-ahead), **location** (city list + "Near me" GPS option).
- **Filter set** (chips + "More filters" panel):
  - Category / sub-category (from the StageX taxonomy)
  - **Age band** (5–8 / 9–12 / 13–16 / 17–21 / 22+)
  - **Fees** (Free / under ₹300 / ₹300–500 / ₹500+ or slider)
  - **Rounds** (1 / 2 / 3+)
  - Mode (On-stage / Online submission), Participation type (Solo/Duet/Group)
  - Date (this week / month / range), Sort (closing soon, nearest, cheapest, popular)
- **Event cards show minimal info by design:** name, category tag, dates + city, rounds + age range, fee, slots-filled bar, status badge, Register + details. Full rules live on the event detail page.
- States: loading skeletons, empty results ("no events match — clear filters / ask the Assistant"), applied-filter chips with ✕ to remove.

---

## 5. Screen C — Registration flow (3 steps)

A compact stepper: **Choose participants → Payment → Confirmed**, headed by an event summary strip.

### 5.1 Step 1 — Choose participants
- **Saved people** appear as selectable cards (photo/initials, name, age, relationship, profile %) with a **per-person category picker** (only categories whose age band matches are offered — eligibility enforced pre-payment).
- **Multi-select:** register one, some, or all; the selected count and subtotal update live (shown in wireframe).
- **Ad-hoc add:** **mandatory** — name*, **date of birth*** (age & age-band auto-derived), **gender***, **Aadhaar number*** (text field, 12 digits, checksum-validated, displayed masked `XXXX-XXXX-1234` after save, stored encrypted), relationship* (Daughter/Son/Niece/Nephew/Myself/Student/Other), category*. **Optional** — school name, city, guru name — collectable in the same form **or updated later from My People** (per review comment R1). Photo/video always come later via My People.
- Validation: duplicate-person warning (same name+age), group entries require min/max group size, age-band mismatch blocks with explanation.

### 5.2 Step 2 — Payment (Razorpay)
- **Order summary:** one line per participant-entry, editable (remove ✕ returns to step 1).
- **Coupon code:** apply → validates against the Ops coupon pool (scope, validity, max uses); success shows saved amount and re-prices; invalid shows inline reason ("expired", "not valid for this event").
- **Methods via Razorpay:** **UPI QR** (scan-to-pay with expiry countdown), **UPI ID / collect request**, **Cards**, **Net-banking / bank transfer** (Razorpay-generated account + reference), Wallets.
- **Failure & retry policy (as specified):**
  - On failure: no amount captured, entries **held for 30 minutes**, clear failure reason shown.
  - **Retry up to 3 times** (attempt counter visible — "attempt 2 of 3"); user may switch methods between attempts.
  - After the 3rd failure: retries lock; entries stay held for the 30-minute window; user is guided to a different method or support. Webhook reconciliation catches "debited but failed" cases and auto-refunds/confirms (idempotent, per PRD SX-NFR-011).

### 5.3 Step 3 — Confirmation
- Success page: amount, receipt number, **entry ID per participant** (#NM-3241…), and share actions: **Email, SMS, WhatsApp, Copy link, Download receipt PDF** (extensible list).
- Entry QR codes and schedule land in My Events; confirmation also auto-sends on the account's default channels.

---

## 6. Screen D — My Events

- **Upcoming events:** each card lists the event's registered participants. Per participant:
  - **Event-scoped photo** upload/update — with the explicit on-screen rule: *"this photo is used only for this event"* (see §2). Other events are unaffected.
  - **Message / note per participant per event** (e.g., "Costume: green & gold, Gate 2 by 8 AM") — free text, editable until event completion, visible to the account and printed on the family's own schedule sheet.
  - Entry ID badge, category, entry QR access.
- **Completed events:**
  - **🏆 View winners** — expands the winners panel (top 3 with scores, link to the full public leaderboard). Old events remain browsable indefinitely.
  - **⭐ Give feedback** deep-links to Screen G.
  - If a family member **won**, the row shows the medal badge and a shortcut to Certificates.

---

## 7. Screen E — My People

- All people under the phone number, each with: photo, name, age, relationship, art form, events count, achievements, profile %.
- **Edit profile:** **mandatory identity fields** — DOB*, gender*, Aadhaar* (masked read-only once verified; re-enter to change) — plus **optional** school name, city, guru name, global photo, intro **video**, bio/achievements — clearly labeled as the *global* profile vs event-scoped photos (§2).
- **Aadhaar handling (DPDP):** captured as text, Verhoeff-checksum validated client-side, transmitted over TLS, stored encrypted at rest, always displayed masked, never included in exports visible to other participants; admin views show it masked with reveal audit-logged.
- Add person here too (same minimal form as ad-hoc add).
- Privacy: minor profiles default to restricted public visibility (PRD SX-FR-123).

## 8. Screen F — Certificates

- **Certificates tab lists every certificate across the family:** event, year, category + age band, **participant name**, **position** (1st Gold / 2nd Silver / 3rd Bronze / Participation), certificate ID, QR-verifiable badge.
- Actions: **⬇ Download PDF** per certificate, Share.
- Filters: by person, winners-only.
- Empty state per person shown in wireframe ("No certificates yet for Diya…").
- Verification: every certificate ID resolves at a public verify URL (PRD SX-FR-181).

## 9. Screen G — Feedback

- Per attended event (selector), **simple rated questions (1–5 stars)**:
  1. Overall experience
  2. Judges & fairness of scoring
  3. Venue, seating & facilities
  4. **Food stalls & refreshments**
  5. Schedule & organisation
  6. **Sponsor booths & activities**
- **Free-text comment** (optional) + **submit anonymously** toggle (default on).
- Rules: one submission per family per event (editable within 7 days); available only after event completion; ratings feed organiser/judge/sponsor dashboards.

## 10. Screen H — Assistant (chat + voice registration)

- A conversational agent that can **search and register**: "Find dance events in Mumbai for Ananya under ₹500" → matched event mini-cards (age/fee checks pre-applied) → quick-reply chips → "Register Ananya for Nritya Mahotsav" → assistant assembles the entry and hands off to the **secure payment step** (it never takes payment inside chat).
- **Voice provision:** mic button (browser speech-to-text); same intents by voice — "Register Kabir for a tabla event in Delhi".
- Safety rails: the assistant always shows what it's about to do and requires **explicit confirmation** before creating any entry; it can only act on the logged-in family's people; every assistant-created registration is tagged `source=assistant` for analytics.
- Reachable from the top tab, the mobile bottom-bar center button, and a "Can't decide? Ask the Assistant" prompt on empty/large search results.

---

## 11. Cross-cutting web specs

- **Responsive:** desktop → tablet → mobile; under 900 px the top tabs collapse into a **bottom navigation bar** (Discover · Events · ✦ Assistant · Certs · People) with safe-area padding; cards stack to single column; OTP boxes resize.
- **Design system:** same StageX identity (Purple #6A1B9A, Gold #FFC107, Navy #0E1237, accent Orange/Pink/Sky/Green; Montserrat display + Inter body + Poppins UI; rounded cards, soft shadows, gradient CTAs, floating labels, status badges, progress bars).
- **States everywhere:** loading, empty, error, success; inline validation with helpful messages.
- **Accessibility:** WCAG 2.1 AA targets — focus-visible on all controls, labels on inputs, contrast-checked palette, keyboard operable stepper and stars, reduced-motion respected.
- **Performance:** card images lazy-loaded; payment page stays functional on 3G (QR renders locally).

### 11.1 Key API touchpoints (illustrative)
`POST /auth/otp/send` · `POST /auth/otp/verify` · `POST /auth/register` · `POST /auth/login` · `GET /events?query&city&filters` · `GET /people` · `POST /people` (minimal ad-hoc) · `PATCH /people/:id` · `POST /registrations` (multi-entry) · `POST /coupons/validate` · `POST /payments/order` (Razorpay order) · `POST /payments/webhook` · `GET /my/events` · `PATCH /entries/:id/event-photo` · `PATCH /entries/:id/note` · `GET /events/:id/winners` · `GET /my/certificates` · `GET /certificates/:id/download` · `POST /feedback` · `POST /assistant/message`

### 11.2 Business rules recap (the ones engineers will ask about)
1. One phone = one family account; people are account-scoped, reusable across events.
2. Person record: **mandatory** = name, DOB, gender, Aadhaar (text, masked, encrypted), relationship (+ category at registration); **optional** = school, city, guru name — at registration or later from My People. Age band derives from DOB.
3. Event photos are **entry-scoped**; global photo is only a fallback; never cross-leak between events.
4. Eligibility (age band × category) validated **before** payment.
5. Coupons come from the Ops pool; single coupon per order (v1).
6. Payment: hold entries 30 min; **max 3 retry attempts**; idempotent webhooks; auto-refund on capture-after-fail.
7. Confirmation shareable via Email / SMS / WhatsApp / link / PDF.
8. Notes are per participant **per event**; feedback is per family per event, post-completion, anonymous by default.
9. Winners of past events are permanently viewable; certificates are QR-verifiable and downloadable per win.
10. Assistant requires explicit confirmation; payment never happens in chat.

---

## 12. Open questions

- **Aadhaar (from R1):** checksum-only validation, or live UIDAI/DigiLocker verification? What's the alternative ID for foreign/NRI participants (passport?) — and do under-5s always have Aadhaar available?
- Should a teen (16–17) be allowed a linked "view-only" login of their own profile?
- Multiple coupons / auto-apply best coupon at checkout?
- Refund UX when one of several participants withdraws from a multi-entry order (partial refund flow).
- Assistant languages at launch (English + Hindi first?), and voice on which browsers.
- Do event-scoped photos require moderation before appearing on judge sheets?

## 13. Demo script (3 minutes)

1. Avatar → **Auth**: New account → phone (see validation) → OTP boxes → set password. (30 s)
2. **Discover**: point at name/location search + fee/rounds/age filter chips; Register on Nritya Mahotsav. (20 s)
3. **Step 1**: toggle Diya on — watch count & subtotal change; show the 20-second ad-hoc add form. (30 s)
4. **Step 2**: Apply EARLYBIRD20 (total drops), show the QR/UPI/bank methods, press **Pay** three times to demo the retry counter and lockout, then "Demo: successful retry". (45 s)
5. **Step 3**: share chips (Email/SMS/WhatsApp/PDF) → Go to My Events. (15 s)
6. **My Events**: save a note for Ananya, update her event-only photo (note the rule), expand **winners** on Taal Tarang. (30 s)
7. **Certificates**: download Kabir's Gold certificate; **Feedback**: click stars incl. food stalls & sponsors, submit; **Assistant**: show the chat registration and tap the mic. (30 s)

---

*End of ClientDesignWeb v1.0 — Participant Web Experience. Mobile App design to follow as a separate document on the same account model and APIs.*
