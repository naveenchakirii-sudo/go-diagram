import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Provider } from 'react-redux';
import { makeStore } from './store';
import App from './components/App';
import '../css/index.styl';

createRoot(document.getElementById('ReactApp')).render(
  <StrictMode>
    <Provider store={makeStore()}>
      <App />
    </Provider>
  </StrictMode>,
);
