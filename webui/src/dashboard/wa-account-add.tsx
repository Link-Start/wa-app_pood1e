import { useEffect, useState } from 'react';
import { CheckCircle2, KeyRound, Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { pollWaAccountTransferRegistration, probeWaPhoneSMS, refreshWaAccountTransferChallenge, registerWaPhone, submitWaRegistrationOTP, type WaWorkflowResponse } from './wa-api';
import { WhatsAppIcon } from './wa-brand-icon';
import { accountReasonLabel } from './wa-result-labels';
import { waProbeStatus } from './wa-result-model';
import { WaRegistrationChannelButtons } from './wa-registration-channel-buttons';
import { WaRegistrationOtpCard, WA_REGISTRATION_OTP_LENGTH } from './wa-registration-otp-card';
import { registrationAnyMethodAvailable, registrationChannelsHardBlocked, type SelectableRegistrationMethodOption } from './wa-registration-methods';
import { WaResultPanel } from './wa-result-panel';
import { resolveWaPhoneTarget, type WaResolvedPhone } from './wa-utils';
type ProbeState = { target: WaResolvedPhone; result: WaWorkflowResponse } | null;
type PendingRegistration = { accountID: string; verificationRequestID: string; accountTransferChallenge?: Record<string, unknown> };
type Props = { disabled?: boolean; onChanged: () => void | Promise<void>; onDone: (message: string) => void; onError: (message: string) => void };
export function WaAccountAdd({ disabled, onChanged, onDone, onError }: Props) {
  const [phone, setPhone] = useState('');
  const [countryCallingCode, setCountryCallingCode] = useState('');
  const [probe, setProbe] = useState<ProbeState>(null);
  const [pending, setPending] = useState<PendingRegistration | null>(null);
  const [registrationResult, setRegistrationResult] = useState<WaWorkflowResponse | null>(null);
  const [registrationTarget, setRegistrationTarget] = useState<WaResolvedPhone | null>(null);
  const [cooldownStartedAt, setCooldownStartedAt] = useState(Date.now());
  const [clockNow, setClockNow] = useState(Date.now());
  const [otp, setOtp] = useState('');
  const [busy, setBusy] = useState(false);
  const samePhone = probeMatchesValues(probe, phone, countryCallingCode);
  const currentTarget = resolveWaPhoneTarget(phone, countryCallingCode).target;
  const registrationSamePhone = Boolean(registrationTarget && currentTarget?.e164 === registrationTarget.e164);
  const activeRegistrationResult = registrationSamePhone ? registrationResult : null;
  const status = waProbeStatus(activeRegistrationResult || (samePhone ? probe?.result : null));
  const channelStatus = samePhone ? waProbeStatus(activeRegistrationResult || probe?.result) : null;
  const cooldownElapsedSeconds = Math.max(0, (clockNow - cooldownStartedAt) / 1000);
  const blocked = status.blocked === true;
  const showChannels = Boolean(channelStatus && !pending);
  const channelsHardBlocked = registrationChannelsHardBlocked(channelStatus);
  const canRegister = samePhone && registrationAnyMethodAvailable(channelStatus, cooldownElapsedSeconds) && !channelsHardBlocked;
  const detected = samePhone && Boolean(channelStatus);
  const badgeVariant = pending ? 'default' : blocked ? 'destructive' : canRegister ? 'default' : detected ? 'secondary' : 'outline';
  const accountTransferPending = Boolean(pending?.accountTransferChallenge);
  const badgeLabel = pending ? accountTransferPending ? '等待迁移' : '等待 OTP' : blocked ? '已封禁' : canRegister ? '可注册' : detected ? '无可直发' : '待检测';

  useEffect(() => {
    const activeResult = activeRegistrationResult || (samePhone ? probe?.result : null);
    if (!activeResult) return undefined;
    const timer = window.setInterval(() => setClockNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [activeRegistrationResult, probe, samePhone]);

  async function runProbe() {
    const resolved = resolveWaPhoneTarget(phone, countryCallingCode);
    if (!resolved.target) return onError(resolved.error || '请输入手机号和国家拨号码');
    setBusy(true);
    try {
      setRegistrationResult(null);
      setRegistrationTarget(null);
      setPending(null);
      const result = await probeWaPhoneSMS(resolved.target.input);
      resetCooldownClock();
      setProbe({ target: resolved.target, result });
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
	  async function submitOTP() {
	    if (!pending) return onError('没有等待中的 OTP');
	    if (pending.accountTransferChallenge) return onError('账号迁移不使用 OTP 输入');
    const code = otp.trim();
    if (!code) return onError('请输入 OTP');
    if (code.length !== WA_REGISTRATION_OTP_LENGTH) return onError(`请输入 ${WA_REGISTRATION_OTP_LENGTH} 位 OTP`);
    setBusy(true);
    try {
      const result = await submitWaRegistrationOTP(pending.accountID, code);
      if (result.success === false || result.error_message) throw new Error(accountReasonLabel(result.error_message, result.status) || 'OTP 提交失败');
      setOtp('');
      setPending(null);
      onDone('OTP 已提交');
      await onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  async function startRegistration(method: SelectableRegistrationMethodOption) {
    const resolved = resolveWaPhoneTarget(phone, countryCallingCode);
    if (!resolved.target) return onError(resolved.error || '请输入手机号和国家拨号码');
    if (!samePhone || !channelStatus) return onError('请先检测');
    setBusy(true);
    try {
      const result = await registerWaPhone(resolved.target.input, method.value);
      const resultStatus = waProbeStatus(result);
      resetCooldownClock();
      setRegistrationResult(result);
      setRegistrationTarget(resolved.target);
      if (result.success === false || result.error_message || resultStatus.blocked === true || resultStatus.requestFailed) {
        onError(registrationFailureMessage(result, resultStatus));
        return;
      }
	      const accountID = workflowText(result, 'wa_account_id');
	      const verificationRequestID = workflowText(result, 'verification_request_id');
	      if (accountID) setPending({ accountID, verificationRequestID, accountTransferChallenge: result.account_transfer_challenge });
	      setProbe(null);
	      setOtp('');
	      onDone(result.account_transfer_challenge ? '账号迁移已发起' : accountID ? 'OTP 已发送' : '已发起');
      await onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  function resetCooldownClock() {
    const now = Date.now();
    setCooldownStartedAt(now);
    setClockNow(now);
  }
  async function refreshAccountTransfer() {
    if (!pending?.verificationRequestID) return onError('缺少验证请求');
    setBusy(true);
    try {
      const result = await refreshWaAccountTransferChallenge(pending.verificationRequestID);
      if (result.success === false || result.error_message) throw new Error(accountReasonLabel(result.error_message, result.status) || '刷新迁移 Deeplink 失败');
      setPending({ ...pending, accountTransferChallenge: result.account_transfer_challenge });
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  async function pollAccountTransfer() {
    if (!pending?.verificationRequestID) return onError('缺少验证请求');
    setBusy(true);
    try {
      const result = await pollWaAccountTransferRegistration(pending.verificationRequestID, pending.accountID, 1);
      if (result.success === false || result.error_message) throw new Error(accountReasonLabel(result.error_message, result.status) || '账号迁移仍在等待确认');
      setPending(null);
      onDone('账号迁移已完成');
      await onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }
  async function copyText(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      onDone('已复制');
    } catch {
      onError('复制失败');
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="grid gap-1">
          <CardTitle className="inline-flex items-center gap-2 text-base"><WhatsAppIcon className="size-5" />添加 WAAccount</CardTitle>
        </div>
        <Badge variant={badgeVariant}>
          {pending ? <KeyRound size={12} /> : canRegister ? <CheckCircle2 size={12} /> : null}
          {badgeLabel}
        </Badge>
      </CardHeader>
      <CardContent className="grid gap-3">
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-[160px_1fr]">
            <Field><FieldLabel>国家拨号码</FieldLabel><Input placeholder="+1" value={countryCallingCode} onChange={(event) => setCountryCallingCode(event.target.value)} disabled={busy || disabled} /></Field>
            <Field>
              <FieldLabel>手机号</FieldLabel>
              <div className="flex gap-2">
                <Input placeholder="4155550123" value={phone} onChange={(event) => setPhone(event.target.value)} disabled={busy || disabled} />
                <Button type="button" size="icon" variant="outline" disabled={busy || disabled} title="检测手机号" aria-label="检测手机号" onClick={() => void runProbe()}><Search size={14} /></Button>
              </div>
            </Field>
          </div>
          {probe && !samePhone && <Badge variant="outline">号码已变化，请重新检测</Badge>}
        </FieldGroup>
        {showChannels && (
          <Field>
            <FieldLabel>通道</FieldLabel>
            <WaRegistrationChannelButtons status={channelStatus} elapsedSeconds={cooldownElapsedSeconds} disabled={busy || disabled || Boolean(pending) || channelsHardBlocked} onStart={(method) => void startRegistration(method)} />
          </Field>
        )}
        {pending && (pending.accountTransferChallenge ? (
          <WaAccountTransferCard
            challenge={pending.accountTransferChallenge}
            busy={busy}
            onCopy={(value) => void copyText(value)}
            onPoll={() => void pollAccountTransfer()}
            onRefresh={() => void refreshAccountTransfer()}
          />
        ) : <WaRegistrationOtpCard value={otp} busy={busy} onChange={setOtp} onSubmit={() => void submitOTP()} />)}
        {(activeRegistrationResult || probe || busy) && (
          <Card className="p-3">
            <WaResultPanel title={activeRegistrationResult ? '注册结果' : '检测结果'} phone={registrationSamePhone ? registrationTarget?.e164 || '' : samePhone ? probe?.target.e164 || '' : ''} result={activeRegistrationResult || (samePhone ? probe?.result || null : null)} loading={busy} showMethods={!showChannels} />
          </Card>
        )}
      </CardContent>
    </Card>
  );
}
function probeMatchesValues(probe: ProbeState, phone: string, countryCallingCode: string) {
  if (!probe) return false;
  return resolveWaPhoneTarget(phone, countryCallingCode).target?.e164 === probe.target.e164;
}
function workflowText(result: WaWorkflowResponse, key: keyof WaWorkflowResponse) {
  const value = result[key];
  return typeof value === 'string' ? value.trim() : '';
}
function WaAccountTransferCard({ challenge, busy, onRefresh, onPoll, onCopy }: { challenge: Record<string, unknown>; busy?: boolean; onRefresh: () => void; onPoll: () => void; onCopy: (value: string) => void }) {
  const deeplink = sensitiveValue(challenge.qr_deeplink);
  return (
    <Card className="border-dashed">
      <CardContent className="grid gap-2 p-3">
        <CardTitle className="inline-flex items-center gap-2 text-sm"><KeyRound size={15} />账号迁移</CardTitle>
        <div className="grid gap-2 text-xs text-muted-foreground">
          <span>第 {textValue(challenge.current_code_index) || '-'} / {textValue(challenge.code_count) || '-'} 个迁移码，按 APK 策略 60s 轮转。</span>
          <Input readOnly type="password" value={deeplink} placeholder="迁移 Deeplink" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" variant="outline" disabled={busy || !deeplink} onClick={() => onCopy(deeplink)}>复制 Deeplink</Button>
          <Button type="button" size="sm" variant="outline" disabled={busy} onClick={onRefresh}>刷新</Button>
          <Button type="button" size="sm" disabled={busy} onClick={onPoll}>检测完成</Button>
        </div>
      </CardContent>
    </Card>
  );
}
function sensitiveValue(value: unknown) {
  const data = typeof value === 'object' && value ? value as Record<string, unknown> : {};
  return textValue(data.value);
}
function textValue(value: unknown) {
  if (typeof value === 'string') return value;
  if (typeof value === 'number') return String(value);
  return '';
}
function registrationFailureMessage(result: WaWorkflowResponse, status: ReturnType<typeof waProbeStatus>) {
  const detail = status.failureReason || result.error_message || result.status || '';
  const reason = accountReasonLabel(detail);
  if (status.blocked) return '号码被拒绝/封禁';
  if (status.accountFlow === 'invalid_number') return reason || '号码无效';
  if (status.accountFlow === 'rate_limited') return reason || '请求冷却中';
  return reason || '注册失败';
}
