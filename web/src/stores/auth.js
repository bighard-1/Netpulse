import { defineStore } from "pinia";
import { api } from "../services/api";

const TOKEN_KEY = "netpulse_token";
const USER_KEY = "netpulse_user";
const SESSION_POLICY_KEY = "netpulse_session_policy";

function loadSessionPolicy() {
  try {
    const data = JSON.parse(localStorage.getItem(SESSION_POLICY_KEY) || "{}");
    return {
      idle_timeout_min: Math.max(5, Math.min(1440, Number(data.idle_timeout_min || 180)))
    };
  } catch {
    return { idle_timeout_min: 180 };
  }
}

export const useAuthStore = defineStore("auth", {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || "",
    user: JSON.parse(localStorage.getItem(USER_KEY) || "null"),
    sessionPolicy: loadSessionPolicy(),
    currentDevice: null
  }),
  getters: {
    isAuthed: (s) => !!s.token,
    isAdmin: (s) => s.user?.role === "admin"
  },
  actions: {
    setAuth(token, user, sessionPolicy = null) {
      this.token = token || "";
      this.user = user || null;
      if (this.token) localStorage.setItem(TOKEN_KEY, this.token);
      else localStorage.removeItem(TOKEN_KEY);
      if (this.user) localStorage.setItem(USER_KEY, JSON.stringify(this.user));
      else localStorage.removeItem(USER_KEY);
      if (sessionPolicy) this.setSessionPolicy(sessionPolicy);
    },
    async login(username, password) {
      const res = await api.login(username, password);
      this.setAuth(res.data?.token, res.data?.user, res.data?.session);
      return res;
    },
    setSessionPolicy(policy) {
      const idle = Math.max(5, Math.min(1440, Number(policy?.idle_timeout_min || 180)));
      this.sessionPolicy = { idle_timeout_min: idle };
      localStorage.setItem(SESSION_POLICY_KEY, JSON.stringify(this.sessionPolicy));
    },
    logout() {
      this.setAuth("", null);
      this.currentDevice = null;
    },
    setCurrentDevice(device) {
      this.currentDevice = device || null;
    }
  }
});
