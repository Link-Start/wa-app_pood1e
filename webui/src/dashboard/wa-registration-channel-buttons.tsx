import { CheckCircle2, Clock3, CircleDashed, PhoneMissed, XCircle } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { countdownLabel } from './wa-result-labels';
import {
  registrationMethodAvailable,
  registrationMethodCooldownSeconds,
  registrationMethodStatus,
  visibleRegistrationChannelMethods,
  type RegistrationChannelMethodOption,
  type SelectableRegistrationMethodOption,
} from './wa-registration-methods';
import type { WaProbeStatus } from './wa-result-model';

type Props = {
  status: WaProbeStatus | null;
  elapsedSeconds: number;
  disabled?: boolean;
  onStart: (method: SelectableRegistrationMethodOption) => void;
};

export function WaRegistrationChannelButtons({ status, elapsedSeconds, disabled, onStart }: Props) {
  const methods = channelMethods(status);
  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {methods.map((method) => {
        const state = channelState(method, status, elapsedSeconds);
        return (
          <Button
            key={method.value}
            type="button"
            variant={state.ready ? 'default' : state.cooldown > 0 ? 'secondary' : 'outline'}
            className="h-10 justify-between gap-3 px-3"
            disabled={disabled || !state.ready}
            aria-label={`${method.label} ${state.label}`}
            title={state.title}
            onClick={() => method.directRequest && onStart(method)}
          >
            <span className="truncate text-sm font-medium">{method.label}</span>
            <Badge variant={state.badge} className="shrink-0">
              <state.Icon />
              {state.label}
            </Badge>
          </Button>
        );
      })}
    </div>
  );
}

function channelMethods(status: WaProbeStatus | null) {
  if (status?.blocked === true) return visibleRegistrationChannelMethods;
  const methods = status ? visibleRegistrationChannelMethods.filter((method) => registrationMethodStatus(status, method.value)) : visibleRegistrationChannelMethods;
  return [...methods.filter((method) => method.directRequest), ...methods.filter((method) => !method.directRequest)];
}

function channelState(method: RegistrationChannelMethodOption, status: WaProbeStatus | null, elapsedSeconds: number) {
  if (!status) return { ready: false, cooldown: 0, label: '先检测', badge: 'outline' as const, Icon: CircleDashed, title: '先检测' };
  if (status.blocked === true) {
    return { ready: false, cooldown: 0, label: '封禁', badge: 'destructive' as const, Icon: XCircle, title: '号码封禁，通道不可用' };
  }
  const cooldown = registrationMethodCooldownSeconds(status, method.value, elapsedSeconds);
  if (cooldown > 0) {
    return { ready: false, cooldown, label: countdownLabel(cooldown), badge: 'secondary' as const, Icon: Clock3, title: '冷却中' };
  }
  if (!method.directRequest) {
    return { ready: false, cooldown: 0, label: '不支持', badge: 'outline' as const, Icon: PhoneMissed, title: '不支持' };
  }
  if (registrationMethodAvailable(status, method.value, elapsedSeconds)) {
    return { ready: true, cooldown: 0, label: '可用', badge: 'default' as const, Icon: CheckCircle2, title: '可用' };
  }
  return { ready: false, cooldown: 0, label: '不可用', badge: 'outline' as const, Icon: XCircle, title: `${method.label} 当前不可用` };
}
