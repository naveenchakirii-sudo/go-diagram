import { configureStore } from '@reduxjs/toolkit';
import diagramReducer, { actions } from './diagramSlice';

// Opens the websocket, feeds server pushes into the store and sends the
// full package list back after every edit. Side effects live here rather
// than in the reducer.
export function createSyncMiddleware(createSocket = defaultSocket) {
  return (store) => {
    let socket = null;
    let retry = 0;

    const connect = () => {
      store.dispatch(actions.connectionChanged('connecting'));
      socket = createSocket();
      socket.onopen = () => {
        retry = 0;
        store.dispatch(actions.connectionChanged('connected'));
      };
      socket.onmessage = (e) => {
        let data;
        try {
          data = JSON.parse(e.data);
        } catch {
          return;
        }
        if (data.error) {
          store.dispatch(actions.serverError(data.error));
        } else {
          store.dispatch(actions.packageDataReceived(data));
        }
      };
      socket.onclose = () => {
        store.dispatch(actions.connectionChanged('disconnected'));
        // Reconnect with backoff, e.g. after restarting the Go server.
        retry = Math.min(retry + 1, 5);
        setTimeout(connect, 500 * 2 ** retry);
      };
    };

    // Defer so the store exists before the first dispatch.
    setTimeout(connect, 0);

    return (next) => (action) => {
      const result = next(action);
      if (action.meta && action.meta.sync && socket && socket.readyState === 1) {
        const { packages } = store.getState().diagram;
        socket.send(JSON.stringify({ packages, edges: [] }));
      }
      return result;
    };
  };
}

function defaultSocket() {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return new WebSocket(`${proto}://${window.location.host}/ws`);
}

export function makeStore(middleware = createSyncMiddleware()) {
  return configureStore({
    reducer: { diagram: diagramReducer },
    middleware: (getDefault) => getDefault().concat(middleware),
  });
}
