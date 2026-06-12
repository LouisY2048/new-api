# Open API Test Supplement + Bug Fixes Design

**Date:** 2026-06-12
**Based on:** 注册+查询接口规范.md V1.1

## Scope

Three areas: test supplementation, backend bug fixes, frontend bug fix.

---

## 1. Test Supplement — Python (test/test_open_api.py)

### Section A: Issue APIKey Boundary Validation

| # | Case | Body | Expected code | Note |
|---|------|------|--------------|------|
| A.1 | Missing iccid | `{"email":"t@t.com","planid":1}` | 10001 | Empty iccid → DB not found |
| A.2 | Missing email (empty) | `{"iccid":"xxx","email":"","planid":1}` | 10002 | Backend must add empty check |
| A.3 | Missing planid | `{"iccid":"xxx","email":"t@t.com"}` | 99999 | planid=0 → plan not found |
| A.4 | Empty body string | `""` | 99999 | `UnmarshalBodyReusable` fails |
| A.5 | Invalid JSON body | `"not json"` | 99999 | Same as above |
| A.6 | ICCID > 20 chars | 30-char ICCID | 10001 | No length validation in code, DB not found |
| A.7 | ICCID < 20 chars | 5-char ICCID | 10001 | Same |

### Section B: Signature Auth Supplement (apikey/issue)

| # | Case | Expected code |
|---|------|--------------|
| B.1 | Non-existent AppId | 10009 |
| B.2 | Disabled App | SKIP (need disable feature first) |
| B.3 | IP whitelist denied | SKIP (need IP whitelist fix first) |

### Section C: Token Usage Boundary Validation

| # | Case | Body | Expected code |
|---|------|------|--------------|
| C.1 | Missing iccid | `{}` | 10001 |
| C.2 | Empty body | `""` | 99999 |
| C.3 | Invalid JSON body | `"bad"` | 99999 |

---

## 2. Backend Bug Fixes

### 2.1 Email empty validation

**File:** `controller/openapi.go` — `IssueApiKey()`
**Change:** Add `len(req.Email) == 0` check → return 10002
**Before email length check** (empty is a format error, not a length error).

### 2.2 Body parse failure → 99999

**File:** `controller/openapi.go` — `IssueApiKey()` and `OpenGetTokenUsage()`
**Change:** `UnmarshalBodyReusable` failure currently returns 10009, change to 99999

### 2.3 IP whitelist separator: comma → newline

**File:** `middleware/open_auth.go` — `OpenAuth()`
**Change:** `strings.Split(app.AllowIps, ",")` → `strings.Split(app.AllowIps, "\n")`
**Also update:** frontend placeholder text in AddEditOpenAppModal.jsx (line 76)

---

## 3. Frontend Bug Fix: Disable Switch Not Working

**File:** `web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx`

**Bug:** The Status Switch uses `defaultChecked={true}` hardcoded (line 68). When editing a disabled app (status=0), initValues sets `status: false`, but `defaultChecked` overrides it — the switch always shows ON. Saving without toggling sends `status: 1`, making disable impossible.

**Fix:** Replace `Form.Slot` + `Switch` with `Form.Switch` (Semi UI native form component):

```jsx
<Form.Switch label='状态' field='status' />
```

Same for IP whitelist toggle (line 71) — also switch to `Form.Switch` for consistency.

### Also: Disable button rendering fix

**File:** `web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx`

**Bug:** The status Tag in the table (line 59-67) uses `status === 1` check but `status` is now controlled by `Form.Switch` which returns boolean. The backend returns integer 0/1. This part is fine (backend returns int).

**No change needed here** — Status column correctly displays 启用/禁用 based on backend int value.
