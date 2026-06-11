import React, { useState } from 'react';
import { Modal, Form, Button, Switch } from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';

const AddEditOpenAppModal = ({ visible, app, onClose, onSuccess }) => {
  const [loading, setLoading] = useState(false);
  const isEdit = !!app;

  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      const payload = {
        id: app?.id || 0,
        app_id: values.app_id || '',
        name: values.name,
        status: values.status ? 1 : 0,
        ip_whitelist_enabled: values.ip_whitelist_enabled,
        allow_ips: values.allow_ips || '',
      };
      const method = isEdit ? API.put : API.post;
      const res = await method('/api/openapp/', payload);
      const { success, data, message } = res.data;
      if (success) {
        showSuccess(isEdit ? '编辑成功' : '新增成功');
        if (!isEdit && data?.app_secret) {
          showSuccess(`AppSecret: ${data.app_secret}（请妥善保管，仅显示一次）`);
        }
        onSuccess();
      } else {
        showError(message || '操作失败');
      }
    } catch (e) {
      showError('操作失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={isEdit ? '编辑开放应用' : '新增开放应用'}
      visible={visible}
      onCancel={onClose}
      footer={null}
    >
      <Form
        onSubmit={handleSubmit}
        initValues={app ? {
          app_id: app.app_id,
          name: app.name,
          status: app.status === 1,
          ip_whitelist_enabled: app.ip_whitelist_enabled,
          allow_ips: app.allow_ips,
        } : {
          status: true,
          ip_whitelist_enabled: false,
        }}
      >
        {!isEdit && (
          <Form.Input field='app_id' label='AppId' placeholder='留空自动生成' />
        )}
        <Form.Input
          field='name'
          label='名称'
          rules={[{ required: true, message: '请输入应用名称' }]}
        />
        <Form.Slot label='状态' field='status'>
          <Switch defaultChecked={true} />
        </Form.Slot>
        <Form.Slot label='IP 白名单开关' field='ip_whitelist_enabled'>
          <Switch />
        </Form.Slot>
        <Form.TextArea
          field='allow_ips'
          label='IP 白名单'
          placeholder='多个 IP 用逗号分隔，如 192.168.1.1,10.0.0.0/24'
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <Button onClick={onClose}>取消</Button>
          <Button type='primary' htmlType='submit' loading={loading}>
            {isEdit ? '保存' : '创建'}
          </Button>
        </div>
      </Form>
    </Modal>
  );
};

export default AddEditOpenAppModal;
