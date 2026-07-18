import {Badge, Box, Button, ContextView, Divider, Inline, List, ListItem, Text} from '@stripe/ui-extension-sdk/ui';
import type {ExtensionContextValue} from '@stripe/ui-extension-sdk/context';

export default function Overview({userContext}: ExtensionContextValue) {
  return (
    <ContextView title="ParityLab" externalLink={{label: 'Open full report', href: 'https://paritylab.dev/demo'}}>
      <Box css={{stack: 'y', gap: 'large'}}>
        <Inline css={{alignY: 'center', distribute: 'space-between'}}>
          <Box>
            <Text weight="semibold">Integration readiness</Text>
            <Text size="small" color="secondary">Sandbox verification</Text>
          </Box>
          <Badge type="positive">92% ready</Badge>
        </Inline>
        <Divider />
        <List>
          <ListItem title="Webhook idempotency" secondaryTitle="Verified" />
          <ListItem title="Event ordering" secondaryTitle="1 finding" />
          <ListItem title="State reconciliation" secondaryTitle="Verified" />
        </List>
        <Button type="primary" onPress={() => undefined}>Run simulation</Button>
        <Text size="small" color="secondary">Signed in as {userContext?.email ?? 'Stripe user'}</Text>
      </Box>
    </ContextView>
  );
}
