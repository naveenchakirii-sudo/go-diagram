import { describe, expect, it, vi } from 'vitest';
import reducer, { actions, initialState } from './diagramSlice';
import { createSyncMiddleware, makeStore } from './store';
import { edgePath } from './refs';

const data = {
  packages: [
    {
      name: 'demo',
      files: [
        {
          name: '/src/demo.go',
          structs: [
            { name: 'Edge', fields: [{ name: 'Weight', type: { literal: 'int', structs: ['int'] } }] },
            { name: 'Node', fields: [] },
          ],
        },
      ],
    },
  ],
  edges: [],
};
const ref = { package: 'demo', file: '/src/demo.go', name: 'Edge' };
const loaded = () => reducer(initialState, actions.packageDataReceived(data));
const edge = (s) => s.packages[0].files[0].structs[0];

describe('diagram reducer', () => {
  it('loads package data without mutating the input', () => {
    const s = loaded();
    expect(edge(s).name).toBe('Edge');
    const s2 = reducer(s, actions.renameStruct({ ...ref, newName: 'Link' }));
    expect(edge(s2).name).toBe('Link');
    expect(edge(s).name).toBe('Edge'); // previous state untouched
    expect(data.packages[0].files[0].structs[0].name).toBe('Edge');
  });

  it('adds structs and fields with unique names', () => {
    let s = loaded();
    s = reducer(s, actions.addStruct({ package: 'demo', file: '/src/demo.go' }));
    s = reducer(s, actions.addStruct({ package: 'demo', file: '/src/demo.go' }));
    const names = s.packages[0].files[0].structs.map((x) => x.name);
    expect(names).toEqual(['Edge', 'Node', 'NewStruct1', 'NewStruct2']);

    s = reducer(s, actions.addField(ref));
    s = reducer(s, actions.addField(ref));
    expect(edge(s).fields.map((f) => f.name)).toEqual(['Weight', 'field1', 'field2']);
  });

  it('renames, retypes and removes fields', () => {
    let s = loaded();
    s = reducer(s, actions.renameField({ ...ref, index: 0, newName: 'Cost' }));
    s = reducer(s, actions.changeFieldType({ ...ref, index: 0, newType: 'float64' }));
    expect(edge(s).fields[0]).toEqual({ name: 'Cost', type: { literal: 'float64', structs: [] } });
    s = reducer(s, actions.removeField({ ...ref, index: 0 }));
    expect(edge(s).fields).toHaveLength(0);
  });

  it('deletes a struct', () => {
    const s = reducer(loaded(), actions.deleteStruct(ref));
    expect(s.packages[0].files[0].structs.map((x) => x.name)).toEqual(['Node']);
  });

  it('ignores edits that reference unknown structs', () => {
    const s = loaded();
    expect(reducer(s, actions.renameStruct({ ...ref, name: 'Nope', newName: 'X' }))).toEqual(s);
  });

  it('only marks edit actions for syncing', () => {
    expect(actions.renameStruct({}).meta).toEqual({ sync: true });
    expect(actions.packageDataReceived(data).meta).toBeUndefined();
  });
});

describe('sync middleware', () => {
  function fakeSocketFactory() {
    const socket = { readyState: 1, send: vi.fn() };
    return { socket, create: () => socket };
  }

  it('sends package data after edits but not after server pushes', async () => {
    vi.useFakeTimers();
    const { socket, create } = fakeSocketFactory();
    const store = makeStore(createSyncMiddleware(create));
    vi.runOnlyPendingTimers(); // opens the socket
    socket.onopen();
    socket.onmessage({ data: JSON.stringify(data) });
    expect(store.getState().diagram.status).toBe('connected');
    expect(socket.send).not.toHaveBeenCalled();

    store.dispatch(actions.renameStruct({ ...ref, newName: 'Link' }));
    expect(socket.send).toHaveBeenCalledTimes(1);
    const sent = JSON.parse(socket.send.mock.calls[0][0]);
    expect(sent.packages[0].files[0].structs[0].name).toBe('Link');
    vi.useRealTimers();
  });

  it('surfaces server errors in state', () => {
    vi.useFakeTimers();
    const { socket, create } = fakeSocketFactory();
    const store = makeStore(createSyncMiddleware(create));
    vi.runOnlyPendingTimers();
    socket.onmessage({ data: JSON.stringify({ error: 'invalid type' }) });
    expect(store.getState().diagram.error).toBe('invalid type');
    vi.useRealTimers();
  });
});

describe('edgePath', () => {
  it('draws a curve from the field to the target struct', () => {
    const d = edgePath(
      { left: 0, right: 100, top: 10, bottom: 40, height: 30 },
      { left: 300, right: 400, top: 0, bottom: 100, height: 100 },
    );
    expect(d.startsWith('M 100 25 C')).toBe(true);
    expect(d.endsWith('300 14')).toBe(true);
  });
});
