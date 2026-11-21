import ws from 'k6/ws';
import { check, sleep } from 'k6';

const base = __ENV.BASE_URL || 'http://localhost:8080';
const tracking = __ENV.TRACKING_NUMBER || 'SDKUWNZTH2U';
const wsBase = base.replace(/^http/, 'ws');

export const options = {
  vus: Number(__ENV.VUS || 5),
  duration: __ENV.DURATION || '15s',
};

export default function () {
  const response = ws.connect(`${wsBase}/ws`, {}, (socket) => {
    socket.on('open', () => {
      socket.send(
        JSON.stringify({ type: 'subscribe', trackingNumber: tracking }),
      );
      socket.setTimeout(() => socket.close(), 5000);
    });
    socket.on('message', (data) => {
      const message = JSON.parse(data);
      check(message, {
        'subscription confirmed': (m) => m.type === 'subscribed',
      });
    });
    socket.on('error', (error) => {
      check(error, { 'websocket error': () => false });
    });
  });
  check(response, { 'websocket upgrade 101': (r) => r && r.status === 101 });
  sleep(1);
}
