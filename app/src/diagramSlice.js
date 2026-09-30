import { createSlice } from '@reduxjs/toolkit';

// State mirrors the server's JSON:
// { packages: [{ name, files: [{ name, structs: [{ name, fields: [{ name, type: { literal, structs }, embedded }] }] }] }],
//   edges: [{ from: Node, to: Node }] }
export const initialState = {
  status: 'connecting', // connecting | connected | disconnected
  error: null,
  packages: [],
  edges: [],
};

function findFile(state, { package: pkgName, file: fileName }) {
  const pkg = state.packages.find((p) => p.name === pkgName);
  return pkg && pkg.files.find((f) => f.name === fileName);
}

function findStruct(state, ref) {
  const file = findFile(state, ref);
  return file && file.structs.find((s) => s.name === ref.name);
}

// Each edit reducer is marked with meta.sync so the sync middleware knows to
// push the resulting package data to the server.
const edit = (reducer) => ({
  reducer,
  prepare: (payload) => ({ payload, meta: { sync: true } }),
});

const slice = createSlice({
  name: 'diagram',
  initialState,
  reducers: {
    connectionChanged(state, { payload }) {
      state.status = payload;
    },
    serverError(state, { payload }) {
      state.error = payload;
    },
    dismissError(state) {
      state.error = null;
    },
    packageDataReceived(state, { payload }) {
      state.packages = payload.packages || [];
      state.edges = payload.edges || [];
    },

    addStruct: edit((state, { payload }) => {
      const file = findFile(state, payload);
      if (!file) return;
      let n = 1;
      while (file.structs.some((s) => s.name === `NewStruct${n}`)) n += 1;
      file.structs.push({ name: `NewStruct${n}`, fields: [] });
    }),
    deleteStruct: edit((state, { payload }) => {
      const file = findFile(state, payload);
      if (!file) return;
      file.structs = file.structs.filter((s) => s.name !== payload.name);
    }),
    renameStruct: edit((state, { payload }) => {
      const struct = findStruct(state, payload);
      if (struct) struct.name = payload.newName;
    }),
    addField: edit((state, { payload }) => {
      const struct = findStruct(state, payload);
      if (!struct) return;
      let n = 1;
      while (struct.fields.some((f) => f.name === `field${n}`)) n += 1;
      struct.fields.push({ name: `field${n}`, type: { literal: 'string', structs: ['string'] } });
    }),
    removeField: edit((state, { payload }) => {
      const struct = findStruct(state, payload);
      if (struct) struct.fields.splice(payload.index, 1);
    }),
    renameField: edit((state, { payload }) => {
      const struct = findStruct(state, payload);
      const field = struct && struct.fields[payload.index];
      if (field) field.name = payload.newName;
    }),
    changeFieldType: edit((state, { payload }) => {
      const struct = findStruct(state, payload);
      const field = struct && struct.fields[payload.index];
      if (field) field.type = { literal: payload.newType, structs: [] };
    }),
  },
});

export const actions = slice.actions;
export default slice.reducer;
