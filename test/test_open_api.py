#!/usr/bin/env python3
"""
Open API 接口测试
覆盖: 签名鉴权 / 入参边界 / 签发APIKey / 查询用量 / 查询边界 / Redis限流 / 响应结构
基于: docs/fangan/注册+查询接口规范.md V1.1
测试计划: docs/fangan/开放接口测试计划.md

用法:
  python3 test/test_open_api.py
  APP_ID=xxx APP_SECRET=yyy BASE_URL=http://host:port python3 test/test_open_api.py
"""

import hashlib
import json
import os
import subprocess
import sys
import time
import urllib.request
import urllib.error

# ============================================================
# Configuration
# ============================================================
BASE_URL = os.environ.get("BASE_URL", "http://localhost:3000")
APP_ID = os.environ.get("APP_ID", "app_L8RtbMiaoi68fANs")
APP_SECRET = os.environ.get("APP_SECRET", "17bff85575ed9db03c501bc81cd6bfab")
PLAN_ID = int(os.environ.get("PLAN_ID", "1"))

TEST_TS = int(time.time())
TEST_EMAIL = f"test-{TEST_TS}@example.com"

# ============================================================
# Helpers
# ============================================================
pass_count = 0
fail_count = 0
skip_count = 0

GREEN = "\033[0;32m"
RED = "\033[0;31m"
YELLOW = "\033[0;33m"
CYAN = "\033[0;36m"
NC = "\033[0m"


def section(name):
    print(f"\n{CYAN}━━━ {name} ━━━{NC}")


def ok(msg):
    global pass_count
    pass_count += 1
    print(f"  {GREEN}PASS{NC} {msg}")


def fail(msg, detail=None):
    global fail_count
    fail_count += 1
    print(f"  {RED}FAIL{NC} {msg}")
    if detail:
        print(f"       {detail}")


def skip(msg):
    global skip_count
    skip_count += 1
    print(f"  {YELLOW}SKIP{NC} {msg}")


def sign(ts: str, body: str) -> str:
    s = APP_ID + APP_SECRET + ts + body
    return hashlib.md5(s.encode()).hexdigest()


def request(path: str, body: str, app_id: str = None,
            ts: str = None, x_sign: str = None,
            omit_headers: list = None):
    """发送 Open API 请求，返回 (响应体dict, http状态码)。

    omit_headers: 要省略的请求头列表，如 ['X-Sign', 'X-AppId']
    """
    # 注意: x_sign 用 None 表示未提供，空字符串 "" 表示故意发送空签名
    if ts is None:
        ts = str(int(time.time()))
    if x_sign is None:
        x_sign = sign(ts, body)
    if app_id is None:
        app_id = APP_ID
    omit_headers = omit_headers or []

    headers = {"Content-Type": "application/json"}
    if "X-AppId" not in omit_headers:
        headers["X-AppId"] = app_id
    if "X-Timestamp" not in omit_headers:
        headers["X-Timestamp"] = ts
    if "X-Sign" not in omit_headers:
        headers["X-Sign"] = x_sign

    req = urllib.request.Request(
        f"{BASE_URL}{path}",
        data=body.encode(),
        headers=headers,
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read().decode()), resp.status
    except urllib.error.HTTPError as e:
        return json.loads(e.read().decode()), e.code
    except Exception as e:
        return {"code": "CONN_ERROR", "message": str(e), "data": None}, 0


def assert_code(resp: dict, expected: str, label: str):
    code = resp.get("code", "NO_CODE")
    if code == expected:
        ok(f"{label} (code={code})")
    else:
        fail(f"{label} — expected code={expected}, got code={code}")


def assert_data_not_null(resp: dict, label: str):
    if resp.get("data") is not None:
        ok(f"{label} — data present")
    else:
        fail(f"{label} — data is null")


def assert_data_null(resp: dict, label: str):
    if resp.get("data") is None:
        ok(f"{label} — data is null")
    else:
        fail(f"{label} — expected null data")


def assert_field(resp: dict, field: str, label: str):
    data = resp.get("data")
    if data is None:
        fail(f"{label} — data is null, expected field '{field}'")
    elif field not in data:
        fail(f"{label} — field '{field}' missing")
    else:
        ok(f"{label} ({field}={data[field]})")


# ============================================================
# Check connectivity
# ============================================================
print("=" * 50)
print(f" Open API 接口测试")
print(f" 基准规范: docs/fangan/注册+查询接口规范.md V1.1")
print(f" Base URL: {BASE_URL}")
print(f" AppId:    {APP_ID}")
print(f" PlanId:   {PLAN_ID}")
print(f" Run ID:   {TEST_TS}")
print("=" * 50)

try:
    urllib.request.urlopen(f"{BASE_URL}/api/status", timeout=5)
    print(f"\n检查服务可达... {GREEN}OK{NC}")
except Exception as e:
    print(f"\n检查服务可达... {RED}FAIL — {BASE_URL} 不可达{NC}")
    sys.exit(1)

REDIS_OK = False
try:
    result = subprocess.run(
        ["docker", "exec", "new-api-redis", "redis-cli", "-a", "cugredis2026", "PING"],
        capture_output=True, text=True, timeout=5
    )
    REDIS_OK = "PONG" in result.stdout
    print(f"检查 Redis 可用... {GREEN if REDIS_OK else YELLOW}{'OK' if REDIS_OK else 'N/A (限流测试将跳过)'}{NC}")
except Exception:
    print(f"检查 Redis 可用... {YELLOW}N/A{NC}")


# ============================================================
# Test data
# ============================================================
# 预置测试用户 (by esim_klJh6TYPXHH5GW9x)
ICCID_ACTIVE = "89860000000000000001"         # 活跃订阅, 至 2026-12-08
# 预置测试用户 (by esim_2cMgcnJzkbmMUUpc)
ICCID_EXPIRED = "finaltest12345678901"        # 订阅已过期, 2026-06-12
# 动态生成
ICCID_NEW = f"88888888888888{TEST_TS % 1000000:06d}"   # 每次运行唯一
ICCID_NOT_EXIST = "99999999999999888888"               # 不存在

# 限流测试 ICCID (末尾6位用于风控判定)
ICCID_RL = "99999999999999222222"
ICCID_EMAIL_1 = "99999999999999100001"
ICCID_EMAIL_2 = "99999999999999200001"
ICCID_EMAIL_3 = "99999999999999300001"
ICCID_EMAIL_4 = "99999999999999400001"
ICCID_EMAIL_5 = "99999999999999500001"
ICCID_EMAIL_6 = "99999999999999600001"
ICCID_CONSEC_1 = "99999999999999500001"
ICCID_CONSEC_2 = "99999999999999500002"
ICCID_CONSEC_3 = "99999999999999500003"
ICCID_CONSEC_4 = "99999999999999500004"
ICCID_CONSEC_5 = "99999999999999500005"

# ============================================================
# Section 1: 签名鉴权 (两个接口通用)
# ============================================================
section("1. 签名鉴权")

AUTH_BODY = '{"iccid":"89860000000000000001","email":"test@test.com","planid":1}'

# 1.1 缺少 X-Sign
resp, _ = request("/open/v1/apikey/issue", AUTH_BODY, x_sign="")
assert_code(resp, "10009", "1.1 缺少 X-Sign")

# 1.2 错误签名
resp, _ = request("/open/v1/apikey/issue", AUTH_BODY, x_sign="0" * 32)
assert_code(resp, "10009", "1.2 错误签名")

# 1.3 过期时间戳 (>300s)
old_ts = str(int(time.time()) - 600)
resp, _ = request("/open/v1/apikey/issue", AUTH_BODY, ts=old_ts)
assert_code(resp, "10009", "1.3 过期时间戳")

# 1.4 缺少 X-AppId
resp, _ = request("/open/v1/apikey/issue", AUTH_BODY, app_id="")
assert_code(resp, "10009", "1.4 缺少 X-AppId")

# 1.5 缺少 X-Timestamp
resp, _ = request("/open/v1/apikey/issue", AUTH_BODY, omit_headers=["X-Timestamp"])
assert_code(resp, "10009", "1.5 缺少 X-Timestamp")

# ============================================================
# Section 2: 签发 APIKey — 参数校验
# ============================================================
section("2. 签发 APIKey — 参数校验")

# 2.1 邮箱 >50 字符
long_email = "a" * 45 + "@b.com"  # 51 chars
body = f'{{"iccid":"89860000000000000001","email":"{long_email}","planid":1}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10002", "2.1 邮箱 >50 字符 → 10002")

# 2.2 planId 不存在 → 99999 (use unique ICCID to avoid test data contamination)
body = f'{{"iccid":"9999999999988{abs(hash(str(TEST_TS))) % 10**7:07d}","email":"t@t.com","planid":-1}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "99999", "2.2 无效 planId → 99999")
assert_data_null(resp, "2.2 data=null")

# ============================================================
# Section 2A: 签发 APIKey — 入参边界校验
# ============================================================
section("2A. 签发 APIKey — 入参边界校验")

# 2A.1 缺少 iccid
body = f'{{"email":"t@t.com","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10001", "2A.1 缺少 iccid → 10001")
assert_data_null(resp, "2A.1 data=null")

# 2A.2 email 为空字符串
body = f'{{"iccid":"88888888888888000001","email":"","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10002", "2A.2 email 为空 → 10002")

# 2A.3 缺少 planid
body = '{"iccid":"88888888888888000002","email":"t2@t.com"}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "99999", "2A.3 缺少 planid → 99999")

# 2A.4 body 为空字符串
resp, _ = request("/open/v1/apikey/issue", "")
assert_code(resp, "99999", "2A.4 body 为空 → 99999")
assert_data_null(resp, "2A.4 data=null")

# 2A.5 body 为非法 JSON
resp, _ = request("/open/v1/apikey/issue", "not json")
assert_code(resp, "99999", "2A.5 body 非法 JSON → 99999")

# 2A.6 ICCID 超过 20 位
body = f'{{"iccid":"{"9" * 30}","email":"t3@t.com","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10001", "2A.6 ICCID >20 位 → 10001")

# 2A.7 ICCID 不足 20 位
body = f'{{"iccid":"{"9" * 5}","email":"t4@t.com","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10001", "2A.7 ICCID <20 位 → 10001")

# ============================================================
# Section 2B: 签名鉴权补充
# ============================================================
section("2B. 签名鉴权补充")

# 2B.1 不存在的 AppId
body = f'{{"iccid":"89860000000000000001","email":"t@t.com","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body, app_id="app_nonexistent_12345678")
assert_code(resp, "10009", "2B.1 不存在 AppId → 10009")

# 2B.2 被禁用的 App（暂时跳过 - 功能待补全）
skip("2B.2 App 状态禁用 → 待功能补全后测试")

# 2B.3 IP白名单拒绝（暂时跳过 - 功能待补全）
skip("2B.3 IP 白名单拒绝 → 待功能补全后测试")

# ============================================================
# Section 3: 签发 APIKey — 正常流程 + 重复签发
# ============================================================
section("3. 签发 APIKey — 正常流程")

ISSUED_ICCID = ""

# 3.1 新 ICCID 首次签发 → 00000
body = f'{{"iccid":"{ICCID_NEW}","email":"{TEST_EMAIL}","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)

if resp.get("code") == "00000":
    assert_code(resp, "00000", "3.1 新 ICCID 首次签发")
    assert_data_not_null(resp, "3.1 data")
    assert_field(resp, "apiKey", "3.1 apiKey")
    assert_field(resp, "tokenTotal", "3.1 tokenTotal")
    assert_field(resp, "validEndDate", "3.1 validEndDate")
    ISSUED_ICCID = ICCID_NEW

    # 3.2 同一 ICCID 重复签发 → 10005 + data
    resp2, _ = request("/open/v1/apikey/issue", body)
    assert_code(resp2, "10005", "3.2 重复签发 → 10005")
    assert_data_not_null(resp2, "3.2 data (含已有 APIKey)")

    # 3.3 ICCID 存在但邮箱不匹配 → 10005 + data=null
    body3 = f'{{"iccid":"{ICCID_NEW}","email":"wrong-{TEST_EMAIL}","planid":{PLAN_ID}}}'
    resp3, _ = request("/open/v1/apikey/issue", body3)
    assert_code(resp3, "10005", "3.3 邮箱不匹配 → 10005")
    assert_data_null(resp3, "3.3 data=null")
else:
    fail("3.1 首次签发失败 — 请确认 PlanId={PLAN_ID} 可用",
         f"code={resp.get('code')} message={resp.get('message')}")

# 3.4 活跃 ICCID 再次签发 → 10005 + data
body = f'{{"iccid":"{ICCID_ACTIVE}","email":"test@test.com","planid":{PLAN_ID}}}'
resp, _ = request("/open/v1/apikey/issue", body)
assert_code(resp, "10005", "3.4 活跃 ICCID 再次签发 → 10005")
assert_data_not_null(resp, "3.4 data (含已有 APIKey)")

# ============================================================
# Section 4: 查询 Token 用量
# ============================================================
section("4. 查询 Token 用量")

# 4.1 token/usage 签名鉴权 (规范要求全接口通用)
body = '{"iccid":"89860000000000000001"}'
resp, _ = request("/open/v1/token/usage", body, x_sign="0" * 32)
assert_code(resp, "10009", "4.1 token/usage 错误签名 → 10009")

# 4.2 不存在的 ICCID → 10001
body = f'{{"iccid":"{ICCID_NOT_EXIST}"}}'
resp, _ = request("/open/v1/token/usage", body)
assert_code(resp, "10001", "4.2 不存在 ICCID → 10001")
assert_data_null(resp, "4.2 data=null")

# 4.3 活跃 ICCID 查询 → 00000 + 完整字段
body = f'{{"iccid":"{ICCID_ACTIVE}"}}'
resp, _ = request("/open/v1/token/usage", body)
assert_code(resp, "00000", "4.3 活跃 ICCID 查询 → 00000")
assert_data_not_null(resp, "4.3 data")
assert_field(resp, "tokenTotal", "4.3 tokenTotal")
assert_field(resp, "tokenUsed", "4.3 tokenUsed")
assert_field(resp, "remainToken", "4.3 remainToken")
assert_field(resp, "validEndDate", "4.3 validEndDate")

# 4.4 验证 remainToken == tokenTotal - tokenUsed
data = resp.get("data")
if data is not None:
    total = data.get("tokenTotal", 0)
    used = data.get("tokenUsed", 0)
    remain = data.get("remainToken", -1)
    if total - used == remain:
        ok(f"4.4 remainToken == tokenTotal - tokenUsed ({remain} = {total} - {used})")
    else:
        fail(f"4.4 remainToken({remain}) != tokenTotal({total}) - tokenUsed({used})")

    # 4.5 usageDetails 是数组
    details = data.get("usageDetails")
    if isinstance(details, list):
        ok("4.5 usageDetails 是数组")
    else:
        fail(f"4.5 usageDetails 不是数组: {type(details).__name__}")
else:
    skip("4.4-4.5 跳过 — 4.3 失败导致 data 为 null")

# 4.6 订阅已过期的 ICCID → 10004
body = f'{{"iccid":"{ICCID_EXPIRED}"}}'
resp, _ = request("/open/v1/token/usage", body)
assert_code(resp, "10004", "4.6 过期 ICCID 查询 → 10004")
assert_data_null(resp, "4.6 data=null")

# ============================================================
# Section 4A: 查询 Token 用量 — 入参边界校验
# ============================================================
section("4A. 查询 Token 用量 — 入参边界校验")

# 4A.1 缺少 iccid
resp, _ = request("/open/v1/token/usage", "{}")
assert_code(resp, "10001", "4A.1 缺少 iccid → 10001")

# 4A.2 body 为空字符串
resp, _ = request("/open/v1/token/usage", "")
assert_code(resp, "99999", "4A.2 body 为空 → 99999")

# 4A.3 body 为非法 JSON
resp, _ = request("/open/v1/token/usage", "bad json")
assert_code(resp, "99999", "4A.3 body 非法 JSON → 99999")

# ============================================================
# Section 5: Redis 限流
# ============================================================
section("5. Redis 限流")

if not REDIS_OK:
    skip("5.x — Redis 不可用，跳过所有限流测试")
else:
    def redis_del(pattern: str):
        subprocess.run(
            ["docker", "exec", "new-api-redis", "sh", "-c",
             f"redis-cli -a cugredis2026 KEYS '{pattern}' 2>/dev/null | xargs -r redis-cli -a cugredis2026 DEL 2>/dev/null"],
            capture_output=True, timeout=5
        )

    # --- 5.1 ICCID 限流 (20次/小时) ---
    print("\n  --- 5.1 ICCID 限流: 同 ICCID 请求 21 次，第 21 次应返回 10008 ---")

    redis_del(f"openapi:ratelimit:iccid:{ICCID_RL}*")

    body = f'{{"iccid":"{ICCID_RL}","email":"rl-{TEST_TS}@test.com","planid":{PLAN_ID}}}'
    blocked_at = -1
    for i in range(1, 22):
        resp, _ = request("/open/v1/apikey/issue", body)
        if resp.get("code") == "10008":
            blocked_at = i
            break

    if blocked_at == 21:
        ok("5.1 第 21 次请求被限流 (code=10008)")
    elif blocked_at > 0:
        fail(f"5.1 第 {blocked_at} 次就被限流，预期第 21 次 (Redis 有残留计数器?)")
    else:
        fail("5.1 21 次请求均未被限流")

    redis_del(f"openapi:ratelimit:iccid:{ICCID_RL}*")

    # --- 5.2 邮箱限流 (5个ICCID/天/邮箱) ---
    print("\n  --- 5.2 邮箱限流: 6 个不同 ICCID 同邮箱，第 6 个应返回 10008 ---")

    EMAIL_LIMIT = f"email-limit-{TEST_TS}@test.com"
    redis_del(f"openapi:ratelimit:email:{EMAIL_LIMIT}*")
    redis_del(f"openapi:iccid_history:{APP_ID}")

    email_iccids = [ICCID_EMAIL_1, ICCID_EMAIL_2, ICCID_EMAIL_3,
                    ICCID_EMAIL_4, ICCID_EMAIL_5, ICCID_EMAIL_6]
    email_blocked = -1
    for idx, iccid in enumerate(email_iccids):
        body = f'{{"iccid":"{iccid}","email":"{EMAIL_LIMIT}","planid":{PLAN_ID}}}'
        resp, _ = request("/open/v1/apikey/issue", body)
        # 每次清理连续 ICCID 历史避免干扰
        redis_del(f"openapi:iccid_history:{APP_ID}")
        if resp.get("code") == "10008":
            email_blocked = idx + 1
            break

    if email_blocked == 6:
        ok("5.2 第 6 个 ICCID 触发邮箱限流 (code=10008)")
    elif email_blocked > 0:
        fail(f"5.2 第 {email_blocked} 个就触发限流，预期第 6 个")
    else:
        fail("5.2 6 个请求均未被邮箱限流")

    redis_del(f"openapi:ratelimit:email:{EMAIL_LIMIT}*")

    # --- 5.3 连续 ICCID 风控 ---
    print("\n  --- 5.3 连续 ICCID 风控: 末尾6位 500001~500005，第5个应返回 10010 ---")

    redis_del(f"openapi:iccid_history:{APP_ID}")

    consec_iccids = [ICCID_CONSEC_1, ICCID_CONSEC_2, ICCID_CONSEC_3,
                     ICCID_CONSEC_4, ICCID_CONSEC_5]
    CONSEC_EMAIL = f"consecutive-{TEST_TS}@test.com"
    consec_blocked = -1
    for idx, iccid in enumerate(consec_iccids):
        body = f'{{"iccid":"{iccid}","email":"{CONSEC_EMAIL}","planid":{PLAN_ID}}}'
        resp, _ = request("/open/v1/apikey/issue", body)
        if idx < 4 and resp.get("code") == "10010":
            fail(f"5.3 第 {idx + 1} 个被误判为连续 (code=10010)")
        if resp.get("code") == "10010":
            consec_blocked = idx + 1
            break

    if consec_blocked == 5:
        ok("5.3 第 5 个连续 ICCID 触发风控 (code=10010)")
    elif consec_blocked > 0:
        fail(f"5.3 第 {consec_blocked} 个触发风控，预期第 5 个")
    else:
        fail("5.3 5 个连续 ICCID 均未触发风控")

    # 清理限流键
    redis_del(f"openapi:iccid_history:{APP_ID}")
    for iccid in consec_iccids + [ICCID_RL]:
        redis_del(f"openapi:ratelimit:iccid:{iccid}*")
    redis_del(f"openapi:ratelimit:email:*{TEST_TS}*")

# ============================================================
# Section 6: 响应结构校验
# ============================================================
section("6. 响应结构校验 (对照规范 V1.1)")

# 6.1 失败响应通用格式: {code: 非00000, message: 非空, data: null}
body = '{"iccid":"non-existent-iccid-xxxx","email":"t@t.com","planid":999}'
resp, _ = request("/open/v1/apikey/issue", body)
code = resp.get("code", "NO_CODE")
if code != "00000":
    ok(f"6.1 失败 code 非 00000 (code={code})")
else:
    fail("6.1 失败 code 为 00000")
if resp.get("message", ""):
    ok("6.1 message 非空")
else:
    fail("6.1 message 为空")
assert_data_null(resp, "6.1 data=null")

# 6.2 成功响应通用格式: {code: 00000, message: 非空, data: 非null}
body = f'{{"iccid":"{ICCID_ACTIVE}"}}'
resp, _ = request("/open/v1/token/usage", body)
assert_code(resp, "00000", "6.2 成功 code=00000")
if resp.get("message"):
    ok("6.2 message 非空")
else:
    fail("6.2 message 为空")
assert_data_not_null(resp, "6.2 data 非 null")

# ============================================================
# Summary
# ============================================================
total = pass_count + fail_count + skip_count
print(f"\n{'=' * 50}")
print(f"  测试结果摘要")
print(f"{'=' * 50}")
print(f"  {GREEN}PASS{NC}: {pass_count}")
print(f"  {RED}FAIL{NC}: {fail_count}")
print(f"  {YELLOW}SKIP{NC}: {skip_count}")
print(f"  TOTAL:  {total}")
print(f"{'=' * 50}")

if fail_count > 0:
    print(f"\n{RED}存在失败用例，请检查上方 FAIL 详情{NC}")
    sys.exit(1)
else:
    print(f"\n{GREEN}全部通过{NC}")
    sys.exit(0)
