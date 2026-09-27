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


def sig(key: str, message: str) -> str:
    """hex(HMAC-SHA256(key, message))"""
    return hmac.new(
        key.encode("utf-8"), message.encode("utf-8"), hashlib.sha256
    ).hexdigest()


def pay_sig(app_key: str, method: str, sign_data: str) -> str:
    return sig(app_key, method + "&" + sign_data)


def signature(session_key: str, sign_data: str) -> str:
    return sig(session_key, sign_data)


def pay_event_sig(app_key: str, event: str, payload: str) -> str:
    return sig(app_key, event + "&" + payload)


CASES = {
    "pay_sig / 普通参数": pay_sig(
        APP_KEY,
        "requestVirtualPayment",
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"platform":"android","productId":"prod_001","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101001","attach":"test"}',
    ),
    "pay_sig / 含 HTML 特殊字符": pay_sig(
        APP_KEY,
        "requestVirtualPayment",
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"platform":"ios","productId":"a<b>&c","goodsPrice":100,'
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
        '"platform":"android","productId":"prod_001","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101001","attach":"test"}',
    ),
    # 与 prepay_test.go 端到端用例同一份 signData（含 HTML 特殊字符）。
    "signature / 端到端(含 HTML 特殊字符)": signature(
        SESSION_KEY,
        '{"offerId":"1234567890","buyQuantity":1,"env":0,"currencyType":"CNY",'
        '"platform":"ios","productId":"a<b>&c","goodsPrice":100,'
        '"outTradeNo":"ORDER20260101002","attach":"x&y"}',
    ),
    "pay_event_sig": pay_event_sig(
        APP_KEY,
        "xpay_goods_deliver_notify",
        '{"outTradeNo":"ORDER20260101001","productId":"prod_001"}',
    ),
}

if __name__ == "__main__":
    for name, value in CASES.items():
        print(f"{name:32s} {value}")
