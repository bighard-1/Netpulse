import { defineStore } from "pinia";
import { api } from "../services/api";

const TOKEN_KEY = "netpulse_token";
const USER_KEY = "netpulse_user";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || "",
    user: JSON.parse(localStorage.getItem(USER_KEY) || "null"),
    currentDevice: null
  }),
  getters: {
    isAuthed: (s) => !!s.token,
    isAdmin: (s) => s.user?.role === "admin"
  },
  actions: {
    setAuth(token, user) {
      this.token = token || "";
      this.user = user || null;
      if (this.token) localStorage.setItem(TOKEN_KEY, this.token);
      else localStorage.removeItem(TOKEN_KEY);
      if (this.user) localStorage.setItem(USER_KEY, JSON.stringify(this.user));
      else localStorage.removeItem(USER_KEY);
    },
    async login(username, password) {
      const res = await api.login(username, password);
      this.setAuth(res.data?.token, res.data?.user);
      return res;
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
