import type { Api } from '@/api/client';
import type { Media, Message, Page } from '@/api/types';
import { ChatController } from '@/chat/controller';
import { formatBytes } from '@/media/format';
import { kindOf, type PickedFile } from '@/media/pick';

import { FakeRealtime, flush } from './fakes';

const photo: PickedFile = {
  uri: 'file:///tmp/beach.jpg',
  name: 'beach.jpg',
  mimeType: 'image/jpeg',
  kind: 'image',
  byteSize: 2048,
  width: 1200,
  height: 800,
};

const emptyPage: Page = { messages: [], next_before: null, next_after: null };

const uploaded = (id: string): Media => ({
  id,
  kind: 'image',
  mime_type: 'image/jpeg',
  byte_size: 2048,
  file_name: 'beach.jpg',
  has_thumbnail: true,
  created_at: '2026-09-06T09:00:00Z',
});

const sentMessage = (attachments: string[]): Message => ({
  id: 'msg_1',
  conversation_id: 'cnv_1',
  sender: { kind: 'user', id: 'usr_1' },
  body: {
    text: 'look',
    attachments: attachments.map((media_id) => ({
      media_id,
      kind: 'image' as const,
      mime_type: 'image/jpeg',
      byte_size: 2048,
      file_name: 'beach.jpg',
      has_thumbnail: true,
    })),
  },
  reply_to: null,
  status: 'complete',
  truncated: false,
  delivery_status: 'pending',
  created_at: '2026-09-06T09:00:01Z',
});

describe('formatBytes', () => {
  it('says what a person reads off a file card', () => {
    expect(formatBytes(0)).toBe('');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2048)).toBe('2.0 KB');
    expect(formatBytes(1468006)).toBe('1.4 MB');
    expect(formatBytes(48 * 1024 * 1024)).toBe('48 MB');
  });
});

describe('kindOf', () => {
  it('sorts a type into what the bubble will draw', () => {
    expect(kindOf('image/png')).toBe('image');
    expect(kindOf('video/mp4')).toBe('video');
    expect(kindOf('audio/mpeg')).toBe('audio');
    expect(kindOf('application/pdf')).toBe('file');
    expect(kindOf('')).toBe('file');
  });
});

describe('sending files', () => {
  it('shows the photo at once, uploads it, then sends the message naming it', async () => {
    const uploads: string[] = [];
    const sends: unknown[] = [];
    const api = {
      listMessages: async () => emptyPage,
      uploadMedia: async (file: { uri: string }) => {
        uploads.push(file.uri);
        return uploaded('med_1');
      },
      sendMessage: async (_id: string, input: unknown) => {
        sends.push(input);
        return sentMessage(['med_1']);
      },
    } as unknown as Api;

    const controller = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1');
    controller.start();
    await flush();

    const sending = controller.send({ text: 'look', files: [photo] });

    // Before the upload finishes, our own bubble is already on screen,
    // drawn from the file on this device.
    const local = controller.getSnapshot().messages[0];
    expect(local?.localKey).toBeTruthy();
    expect(local?.body.attachments?.[0]).toMatchObject({
      local_uri: 'file:///tmp/beach.jpg',
      kind: 'image',
      width: 1200,
      media_id: '',
    });

    await sending;
    expect(uploads).toEqual(['file:///tmp/beach.jpg']);
    expect(sends).toEqual([{ text: 'look', attachments: ['med_1'], action: undefined, reply_to: undefined }]);

    const confirmed = controller.getSnapshot().messages[0];
    expect(confirmed?.localKey).toBeUndefined();
    expect(confirmed?.body.attachments?.[0]?.media_id).toBe('med_1');
  });

  it('does not upload the same photo twice when a failed send is retried', async () => {
    let uploads = 0;
    let failSend = true;
    const api = {
      listMessages: async () => emptyPage,
      uploadMedia: async () => {
        uploads++;
        return uploaded('med_1');
      },
      sendMessage: async () => {
        if (failSend) throw new Error('down');
        return sentMessage(['med_1']);
      },
    } as unknown as Api;

    const controller = new ChatController(api, new FakeRealtime(), 'cnv_1', 'usr_1');
    controller.start();
    await flush();

    await controller.send({ text: 'look', files: [photo] });
    const failed = controller.getSnapshot().messages[0];
    expect(failed?.delivery_status).toBe('failed');
    expect(uploads).toBe(1);

    failSend = false;
    await controller.retry(failed?.localKey as string);

    // The photo was already on the hub; the retry only sent the message.
    expect(uploads).toBe(1);
    expect(controller.getSnapshot().messages[0]?.body.attachments?.[0]?.media_id).toBe('med_1');
  });
});
