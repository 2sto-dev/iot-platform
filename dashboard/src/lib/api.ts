import axios, { type AxiosInstance } from "axios";

function attachAuth(instance: AxiosInstance) {
  instance.interceptors.request.use((config) => {
    const token = localStorage.getItem("access_token");
    if (token) config.headers.Authorization = `Bearer ${token}`;
    return config;
  });

  instance.interceptors.response.use(
    (r) => r,
    async (error) => {
      if (error.response?.status === 401) {
        const refresh = localStorage.getItem("refresh_token");
        if (refresh) {
          try {
            const { data } = await axios.post("/api/token/refresh/", { refresh });
            localStorage.setItem("access_token", data.access);
            error.config.headers.Authorization = `Bearer ${data.access}`;
            return instance.request(error.config);
          } catch {
            localStorage.clear();
            window.location.href = "/login";
          }
        } else {
          localStorage.clear();
          window.location.href = "/login";
        }
      }
      return Promise.reject(error);
    }
  );
}

export const api = axios.create({ baseURL: "/api" });
export const goApi = axios.create({ baseURL: "/go" });

attachAuth(api);
attachAuth(goApi);
