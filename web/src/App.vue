<script setup>
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import DeviceDetail from "./components/DeviceDetail.vue";
import { api } from "./services/api";

const loading = ref(false);
const devices = ref([]);
const selectedId = ref(null);

const addVisible = ref(false);
const addForm = ref({
  ip: "",
  brand: "Huawei",
  community: "public",
  remark: ""
});

const remarkVisible = ref(false);
const remarkForm = ref({ remark: "" });

const currentDevice = computed(
  () => devices.value.find((d) => d.id === selectedId.value) || null
);

async function loadDevices() {
  loading.value = true;
  try {
    const res = await api.listDevices();
    devices.value = res.data || [];
    if (!selectedId.value && devices.value.length) selectedId.value = devices.value[0].id;
    if (selectedId.value && !devices.value.some((d) => d.id === selectedId.value)) {
      selectedId.value = devices.value[0]?.id || null;
    }
  } finally {
    loading.value = false;
  }
}

async function onAddDevice() {
  await api.addDevice(addForm.value);
  addVisible.value = false;
  addForm.value = { ip: "", brand: "Huawei", community: "public", remark: "" };
  ElMessage.success("Device added");
  await loadDevices();
}

async function onDeleteDevice(id) {
  await api.deleteDevice(id);
  ElMessage.success("Device deleted");
  await loadDevices();
}

function openRemarkEdit() {
  if (!currentDevice.value) return;
  remarkForm.value.remark = currentDevice.value.remark || "";
  remarkVisible.value = true;
}

async function saveRemark() {
  await api.updateDeviceRemark(currentDevice.value.id, remarkForm.value.remark);
  remarkVisible.value = false;
  ElMessage.success("Remark updated");
  await loadDevices();
}

async function onBackup() {
  const res = await api.downloadBackup();
  const blobUrl = URL.createObjectURL(new Blob([res.data]));
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = "netpulse_backup.sql.gz";
  a.click();
  URL.revokeObjectURL(blobUrl);
}

async function onRestore(file) {
  await api.restoreFromFile(file.raw);
  ElMessage.success("Restore done");
  await loadDevices();
}

onMounted(loadDevices);
</script>

<template>
  <div class="min-h-screen bg-slate-100">
    <header class="border-b bg-white">
      <div class="mx-auto flex max-w-[1400px] items-center justify-between px-4 py-3">
        <h1 class="text-xl font-bold text-slate-800">NetPulse Web Console</h1>
        <div class="flex items-center gap-2">
          <el-button type="primary" @click="addVisible = true">Add Device</el-button>
          <el-button @click="onBackup">Download Backup</el-button>
          <el-upload :auto-upload="false" :show-file-list="false" accept=".gz" @change="onRestore">
            <el-button>Restore From File</el-button>
          </el-upload>
        </div>
      </div>
    </header>

    <main class="mx-auto grid max-w-[1400px] grid-cols-12 gap-4 p-4">
      <section class="col-span-12 lg:col-span-4">
        <el-card>
          <template #header>
            <div class="flex items-center justify-between">
              <span>Dashboard</span>
              <el-button size="small" @click="loadDevices">Refresh</el-button>
            </div>
          </template>
          <el-table v-loading="loading" :data="devices" @row-click="(row) => (selectedId = row.id)">
            <el-table-column label="Status" width="80">
              <template #default="{ row }">
                <span
                  class="inline-block h-2.5 w-2.5 rounded-full"
                  :class="row.status === 'online' ? 'bg-emerald-500' : 'bg-rose-500'"
                />
              </template>
            </el-table-column>
            <el-table-column prop="ip" label="IP" min-width="130" />
            <el-table-column prop="brand" label="Brand" width="100" />
            <el-table-column prop="remark" label="Remark" min-width="140" />
            <el-table-column label="Action" width="90">
              <template #default="{ row }">
                <el-button type="danger" text @click.stop="onDeleteDevice(row.id)">Delete</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </section>

      <section class="col-span-12 lg:col-span-8">
        <el-empty v-if="!currentDevice" description="Select a device to view details" />
        <div v-else class="space-y-3">
          <div class="flex justify-end">
            <el-button @click="openRemarkEdit">Edit Remark</el-button>
          </div>
          <DeviceDetail :device="currentDevice" />
        </div>
      </section>
    </main>

    <el-dialog v-model="addVisible" title="Add Device" width="520">
      <el-form label-position="top">
        <el-form-item label="IP">
          <el-input v-model="addForm.ip" placeholder="192.168.1.1" />
        </el-form-item>
        <el-form-item label="Brand">
          <el-select v-model="addForm.brand">
            <el-option label="Huawei" value="Huawei" />
            <el-option label="H3C" value="H3C" />
          </el-select>
        </el-form-item>
        <el-form-item label="Community">
          <el-input v-model="addForm.community" placeholder="public" />
        </el-form-item>
        <el-form-item label="Remark">
          <el-input v-model="addForm.remark" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">Cancel</el-button>
        <el-button type="primary" @click="onAddDevice">Save</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="remarkVisible" title="Edit Device Remark" width="480">
      <el-input v-model="remarkForm.remark" type="textarea" :rows="4" />
      <template #footer>
        <el-button @click="remarkVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveRemark">Update</el-button>
      </template>
    </el-dialog>
  </div>
</template>

