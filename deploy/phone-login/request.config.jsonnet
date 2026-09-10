// Request-config template for Kratos's "sms" courier channel.
// ctx here is the httpDataModel Kratos builds per dispatch (see
// courier/http_channel.go in src/kratos): { recipient, body, template_type,
// template_data, ... }. template_data is the marshaled SMS template model
// (courier/template/sms/*.go), which carries the raw OTP under a
// method-specific key (login_code / registration_code / recovery_code /
// verification_code) depending on which flow triggered the send.
//
// We deliberately do NOT forward Aliyun credentials or call Aliyun from
// here - Jsonnet can't compute Aliyun's HMAC-SHA1 request signature. This
// just relays {to, code} to shared/go/api/auth's HandleSMSCourierRelay,
// which does the actual signed Aliyun SendSms call.
function(ctx) {
  to: ctx.recipient,
  code:
    if "login_code" in ctx.template_data then ctx.template_data.login_code
    else if "registration_code" in ctx.template_data then ctx.template_data.registration_code
    else if "recovery_code" in ctx.template_data then ctx.template_data.recovery_code
    else if "verification_code" in ctx.template_data then ctx.template_data.verification_code
    else null,
}
