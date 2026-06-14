import { KeyRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';

type Props = {
  challenge: Record<string, unknown>;
  busy?: boolean;
  onRefresh: () => void;
  onPoll: () => void;
  onCopy: (value: string) => void;
};

export function WaRegistrationDeviceTransferCard({ challenge, busy, onRefresh, onPoll, onCopy }: Props) {
  const deeplink = sensitiveValue(challenge.qr_deeplink);
  return (
    <Card className="border-dashed">
      <CardContent className="grid gap-2 p-3">
        <CardTitle className="inline-flex items-center gap-2 text-sm"><KeyRound size={15} />设备转移</CardTitle>
        <div className="grid gap-2 text-xs text-muted-foreground">
          <span>第 {textValue(challenge.current_code_index) || '-'} / {textValue(challenge.code_count) || '-'} 个转移码，按 APK 策略 60s 轮转。</span>
          <Input readOnly type="password" value={deeplink} placeholder="设备转移 Deeplink" />
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
