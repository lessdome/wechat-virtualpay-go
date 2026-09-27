#!/usr/bin/env python3
"""
生成测试向量。

这些向量是 Go 单元测试里期望值的来源。用独立实现（Python 标准库 hmac）
计算，再由 Go 代码复现，双方互相印证 —— 避免"用同一个错误的实现自证正确"。

用法：
    python3 scripts/gen_vectors.py
然后把输出与 *_test.go 中的 want 字段比对。
"""

import hmac
import hashlib

APP_KEY = "test_app_key_1234567890"
SESSION_KEY = "test_session_key_abcdef"

# ── 官方样例（微信文档正文里公开的假值，用于校验本脚本算法与官方一致）──────────
OFFICIAL_APP_KEY = "12345"
OFFICIAL_SESSION_KEY = "9hAb/NEYUlkaMBEsmFgzig=="
OFFICIAL_POST_BODY = '{"openid": "xxx", "user_ip": "127.0.0.1", "env": 0}'

# 官方对上面这组输入给出的期望值（文档里以 assert 形式写明）。
OFFICIAL_WANT_PAY_SIG = "c37809f27c6d7fd1837ad2500a04512b66b34fd793a39a385fade56dca89a4b5"
OFFICIAL_WANT_SIGNATURE = "089d9e8dc5d308977360c4b79ec600a93d736802802a807d634192328032f6c7"


def sig(key: str, message: str) -> str:
    """hex(HMAC-SHA256(key, message))"""
    return hmac.new(
        key.encode("utf-8"), message.encode("utf-8"), hashlib.sha256
    ).hexdigest()


def pay_sig(app_key: str, method: str, sign_data: str) -> str:
    return sig(app_key, method + "&" + sign_data)


def signature(session_key: str, sign_data: str) -> str:
    return sig(session_key, sign_data)


CASES = {
    "pay_sig / 普通参数": pay_sig(
        APP_KEY,
        "requestVirtualPayment",
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"productId":"prod_001","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101001","attach":"test"}',
    ),
    "pay_sig / 含 HTML 特殊字符": pay_sig(
        APP_KEY,
        "requestVirtualPayment",
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"productId":"a<b>&c","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101002","attach":"x&y"}',
    ),
    "pay_sig / 道具上传(method 为路径)": pay_sig(
        APP_KEY,
        "/xpay/start_upload_goods",
        '{"productId":"prod_001","price":100}',
    ),
    "signature": signature(
        SESSION_KEY,
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"productId":"prod_001","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101001","attach":"test"}',
    ),
    # 与 prepay_test.go 端到端用例同一份 signData（含 HTML 特殊字符）。
    "signature / 端到端(含 HTML 特殊字符)": signature(
        SESSION_KEY,
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"productId":"a<b>&c","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101002","attach":"x&y"}',
    ),
    # ── 官方向量 ──────────────────────────────────────────────────────────
    # 来自微信官方文档正文里**自带 assert** 的样例，是权威来源。
    # 注意 post_body 里带空格：Go 的 json.Marshal 永远产不出这个串，
    # 所以只能原样硬编码 —— 这也正说明「签名串必须与发送串字节级一致」。
    "官方样例 / pay_sig": pay_sig(
        OFFICIAL_APP_KEY,
        "/xpay/query_user_balance",
        OFFICIAL_POST_BODY,
    ),
    "官方样例 / signature": signature(
        OFFICIAL_SESSION_KEY,
        OFFICIAL_POST_BODY,
    ),
}

def main() -> None:
    # 先对着**官方给的期望值**自检：这一步不过，说明本脚本的算法与微信不一致，
    # 后面所有自生成向量都不可信。
    got_pay_sig = CASES["官方样例 / pay_sig"]
    got_signature = CASES["官方样例 / signature"]
    assert got_pay_sig == OFFICIAL_WANT_PAY_SIG, (
        f"pay_sig 与官方不符:\n  得到 {got_pay_sig}\n  期望 {OFFICIAL_WANT_PAY_SIG}"
    )
    assert got_signature == OFFICIAL_WANT_SIGNATURE, (
        f"signature 与官方不符:\n  得到 {got_signature}\n  期望 {OFFICIAL_WANT_SIGNATURE}"
    )
    print("✓ 官方样例校验通过（本脚本算法与微信一致）\n")

    for name, value in CASES.items():
        print(f"{name:32s} {value}")


if __name__ == "__main__":
    main()
