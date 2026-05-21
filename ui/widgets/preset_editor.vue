<!--
Copyright (C) 2026 Snuffy2
SPDX-License-Identifier: AGPL-3.0-only
-->

<template>
  <form
    id="preset-editor"
    class="form1"
    action="javascript:;"
    @submit.prevent="save"
  >
    <h2>{{ mode === "create" ? "Save preset" : "Edit preset" }}</h2>

    <fieldset id="preset-editor-fields" :disabled="submitting">
      <label class="field">
        Preset name
        <input v-model="localState.title" type="text" autocomplete="off" />
      </label>

      <label class="field">
        Type
        <select v-model="localState.type">
          <option value="SSH">SSH</option>
          <option value="Mosh">Mosh</option>
          <option value="ET">ET</option>
          <option value="Telnet">Telnet</option>
        </select>
      </label>

      <label class="field">
        Host
        <input v-model="localState.host" type="text" autocomplete="off" />
      </label>

      <label class="field">
        Tab color
        <input v-model="localState.tabColor" type="color" autocomplete="off" />
      </label>

      <label v-if="usesUser" class="field">
        User
        <input v-model="localState.meta.User" type="text" autocomplete="off" />
      </label>

      <label v-if="usesAuthentication" class="field">
        Authentication
        <select v-model="localState.meta.Authentication">
          <option
            v-for="option in authenticationOptions"
            :key="option"
            :value="option"
          >
            {{ option }}
          </option>
        </select>
      </label>

      <label v-if="usesPassword" class="field horizontal">
        <input v-model="localState.savePassword" type="checkbox" />
        {{
          localState.hasSavedPassword ? "Keep saved password" : "Save password"
        }}
      </label>

      <label v-if="usesPassword && localState.savePassword" class="field">
        {{ localState.hasSavedPassword ? "Replacement password" : "Password" }}
        <input
          v-model="localState.password"
          type="password"
          autocomplete="off"
        />
      </label>

      <label v-if="usesPrivateKey" class="field horizontal">
        <input v-model="localState.savePrivateKey" type="checkbox" />
        Save private key
      </label>

      <div v-if="usesPrivateKey && localState.savePrivateKey" class="field">
        Private key source
        <label class="field horizontal">
          <input
            v-model="localState.privateKeyMode"
            type="radio"
            value="existing"
          />
          Existing server key
        </label>
        <label class="field horizontal">
          <input
            v-model="localState.privateKeyMode"
            type="radio"
            value="upload"
          />
          Upload local key
        </label>
        <label class="field horizontal">
          <input
            v-model="localState.privateKeyMode"
            type="radio"
            value="paste"
          />
          Paste key
        </label>
      </div>

      <label
        v-if="
          usesPrivateKey &&
          localState.savePrivateKey &&
          localState.privateKeyMode === 'existing'
        "
        class="field"
      >
        Server key
        <select v-model="localState.privateKeyFile">
          <option value="">Select a private key</option>
          <option
            v-if="showCurrentPrivateKeyReference"
            :value="localState.privateKeyFile"
          >
            {{ privateKeyFileLabel(localState.privateKeyFile) }}
          </option>
          <option
            v-for="keyFile in privateKeyFiles"
            :key="keyFile"
            :value="keyFile"
          >
            {{ privateKeyFileLabel(keyFile) }}
          </option>
        </select>
      </label>

      <label
        v-if="
          usesPrivateKey &&
          localState.savePrivateKey &&
          localState.privateKeyMode === 'upload'
        "
        class="field"
      >
        Private key file
        <input type="file" autocomplete="off" @change="importPrivateKeyFile" />
      </label>

      <label
        v-if="
          usesPrivateKey &&
          localState.savePrivateKey &&
          (localState.privateKeyMode === 'upload' ||
            localState.privateKeyMode === 'paste')
        "
        class="field"
      >
        Private Key
        <textarea v-model="localState.privateKey" autocomplete="off"></textarea>
      </label>

      <label class="field">
        Encoding
        <input
          v-model="localState.meta.Encoding"
          type="text"
          autocomplete="off"
          placeholder="utf-8"
        />
      </label>

      <label v-if="localState.type === 'Mosh'" class="field">
        Mosh Server
        <input
          v-model="localState.meta['Mosh Server']"
          type="text"
          autocomplete="off"
          placeholder="mosh-server"
        />
      </label>

      <label v-if="localState.type === 'ET'" class="field">
        ET Server Port
        <input
          v-model="localState.meta['ET Server Port']"
          type="text"
          autocomplete="off"
          placeholder="2022"
        />
      </label>

      <label v-if="localState.type === 'ET'" class="field">
        ET Command
        <input
          v-model="localState.meta['ET Command']"
          type="text"
          autocomplete="off"
          placeholder="et"
        />
      </label>

      <div v-if="error.length > 0" id="preset-editor-error">{{ error }}</div>

      <div v-if="confirmingDelete" id="preset-editor-confirm-delete">
        <p>Delete preset "{{ localState.title }}"?</p>
        <button type="button" @click="confirmDelete">Delete preset</button>
        <button
          type="button"
          class="secondary preset-editor-cancel-action"
          @click="confirmingDelete = false"
        >
          Cancel
        </button>
      </div>

      <div v-else-if="promptingAdminKey" id="preset-editor-admin-key">
        <label class="field">
          ShellPort AdminKey
          <input
            v-model="adminKey"
            type="password"
            autocomplete="off"
            autofocus
          />
        </label>
        <div v-if="adminKeyError.length > 0" id="preset-editor-admin-key-error">
          {{ adminKeyError }}
        </div>
        <button type="button" @click="submitAdminKey">Continue</button>
        <button type="button" class="secondary" @click="cancelAdminKeyPrompt">
          Cancel
        </button>
      </div>

      <div v-else class="field preset-editor-actions">
        <button type="submit">Save</button>
        <button v-if="mode === 'edit'" type="button" @click="requestDelete">
          Delete
        </button>
        <button
          type="button"
          class="secondary preset-editor-cancel-action"
          @click="$emit('cancel')"
        >
          Cancel
        </button>
      </div>
    </fieldset>
  </form>
</template>

<script>
import "./preset_editor.css";

import {
  authenticationOptionsForType,
  buildPresetConfigFromEditorState,
  cloneEditorState,
  privateKeyFileLabel,
  requiresAdminKey,
} from "../preset_management.js";

export default {
  props: {
    mode: {
      type: String,
      default: "create",
    },
    state: {
      type: Object,
      required: true,
    },
    policy: {
      type: Object,
      default: () => null,
    },
    adminKeyCached: {
      type: Boolean,
      default: false,
    },
    savePreset: {
      type: Function,
      required: true,
    },
    deletePreset: {
      type: Function,
      required: true,
    },
    privateKeyFiles: {
      type: Array,
      default: () => [],
    },
  },
  emits: ["cancel"],
  data() {
    const localState = cloneEditorState(this.state);
    if (!localState.meta) {
      localState.meta = {};
    }

    return {
      localState,
      submitting: false,
      error: "",
      promptingAdminKey: false,
      confirmingDelete: false,
      pendingAction: null,
      adminKey: "",
      adminKeyError: "",
    };
  },
  computed: {
    showCurrentPrivateKeyReference() {
      return (
        this.localState.privateKeyFile.length > 0 &&
        !this.privateKeyFiles.includes(this.localState.privateKeyFile)
      );
    },
    authenticationOptions() {
      return authenticationOptionsForType(this.localState.type);
    },
    usesAuthentication() {
      return ["SSH", "Mosh", "ET"].includes(this.localState.type);
    },
    usesUser() {
      return ["SSH", "Mosh", "ET"].includes(this.localState.type);
    },
    usesPassword() {
      return (
        this.usesAuthentication &&
        this.localState.meta.Authentication === "Password"
      );
    },
    usesPrivateKey() {
      return (
        this.usesAuthentication &&
        this.localState.meta.Authentication === "Private Key"
      );
    },
  },
  watch: {
    "localState.type"() {
      this.normalizeAuthentication();
    },
    "localState.meta.Authentication"() {
      this.normalizeAuthentication();
    },
  },
  methods: {
    privateKeyFileLabel,
    normalizeAuthentication() {
      if (this.authenticationOptions.length === 0) {
        delete this.localState.meta.Authentication;
        return;
      }

      if (
        !this.authenticationOptions.includes(
          this.localState.meta.Authentication,
        )
      ) {
        this.localState.meta.Authentication = this.authenticationOptions[0];
      }
    },
    async runProtected(action) {
      this.error = "";
      if (
        requiresAdminKey(this.policy) &&
        !this.adminKeyCached &&
        this.adminKey.length <= 0
      ) {
        this.pendingAction = action;
        this.promptingAdminKey = true;
        this.adminKeyError = "";
        return;
      }

      this.submitting = true;
      try {
        await action(this.adminKey);
        this.adminKey = "";
      } catch (e) {
        this.error = String(e);
      } finally {
        this.submitting = false;
      }
    },
    save() {
      const config = buildPresetConfigFromEditorState(this.localState);
      return this.runProtected((adminKey) =>
        this.savePreset({
          config,
          state: this.localState,
          adminKey,
        }),
      );
    },
    requestDelete() {
      this.confirmingDelete = true;
    },
    confirmDelete() {
      this.confirmingDelete = false;
      return this.runProtected((adminKey) =>
        this.deletePreset({
          id: this.localState.id,
          state: this.localState,
          adminKey,
        }),
      );
    },
    importPrivateKeyFile(event) {
      const fileInput = event.target;
      if (!fileInput.files || fileInput.files.length <= 0) {
        return;
      }
      const reader = new FileReader();
      reader.onload = () => {
        this.localState.privateKey = String(reader.result || "");
      };
      reader.readAsText(fileInput.files[0], "utf-8");
    },
    submitAdminKey() {
      const action = this.pendingAction;
      if (action === null) {
        return;
      }

      this.error = "";
      this.adminKeyError = "";
      this.submitting = true;
      return action(this.adminKey)
        .then(() => {
          this.pendingAction = null;
          this.promptingAdminKey = false;
          this.adminKey = "";
        })
        .catch((e) => {
          this.adminKeyError = String(e);
        })
        .finally(() => {
          this.submitting = false;
        });
    },
    cancelAdminKeyPrompt() {
      this.promptingAdminKey = false;
      this.adminKeyError = "";
      this.pendingAction = null;
      this.adminKey = "";
    },
  },
};
</script>
