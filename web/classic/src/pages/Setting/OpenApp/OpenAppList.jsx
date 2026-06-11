import React, { useState } from 'react';
import { Button, Table, Tag, Popconfirm, Space } from '@douyinfe/semi-ui';
import { IconEdit, IconDelete, IconPlus, IconKey, IconRefresh } from '@douyinfe/semi-icons';
import AddEditOpenAppModal from './AddEditOpenAppModal';
import { API, showError, showSuccess } from '../../../helpers';

const OpenAppList = ({ apps, total, page, pageSize, onPageChange, onRefresh }) => {
  const [modalVisible, setModalVisible] = useState(false);
  const [editingApp, setEditingApp] = useState(null);

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/openapp/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess('删除成功');
        onRefresh();
      } else {
        showError(message || '删除失败');
      }
    } catch (e) {
      showError('删除失败');
    }
  };

  const handleViewKey = async (id) => {
    try {
      const res = await API.post(`/api/openapp/${id}/key`);
      const { success, data, message } = res.data;
      if (success && data?.app_secret) {
        showSuccess(`AppSecret: ${data.app_secret}`);
      } else {
        showError(message || '获取失败');
      }
    } catch (e) {
      showError('获取失败');
    }
  };

  const handleRefreshKey = async (id) => {
    try {
      const res = await API.post(`/api/openapp/${id}/key?action=refresh`);
      const { success, data, message } = res.data;
      if (success && data?.app_secret) {
        showSuccess(`新 AppSecret: ${data.app_secret}`);
        onRefresh();
      } else {
        showError(message || '刷新失败');
      }
    } catch (e) {
      showError('刷新失败');
    }
  };

  const columns = [
    { title: 'AppId', dataIndex: 'app_id', key: 'app_id' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={status === 1 ? 'green' : 'red'}>
          {status === 1 ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: 'IP 白名单开关',
      dataIndex: 'ip_whitelist_enabled',
      key: 'ip_whitelist_enabled',
      render: (val) => (val ? '开启' : '关闭'),
    },
    { title: 'IP 白名单', dataIndex: 'allow_ips', key: 'allow_ips' },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (ts) => ts ? new Date(ts * 1000).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button
            icon={<IconEdit />}
            size='small'
            onClick={() => { setEditingApp(record); setModalVisible(true); }}
          />
          <Button
            icon={<IconKey />}
            size='small'
            onClick={() => handleViewKey(record.id)}
          />
          <Button
            icon={<IconRefresh />}
            size='small'
            onClick={() => handleRefreshKey(record.id)}
          />
          <Popconfirm
            title='确定删除该应用？'
            onConfirm={() => handleDelete(record.id)}
          >
            <Button icon={<IconDelete />} size='small' type='danger' />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <h3>开放应用设置</h3>
        <Button
          icon={<IconPlus />}
          theme='solid'
          onClick={() => { setEditingApp(null); setModalVisible(true); }}
        >
          新增应用
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={apps}
        rowKey='id'
        pagination={{
          currentPage: page + 1,
          pageSize,
          total,
          onChange: (p) => onPageChange(p - 1),
        }}
      />
      <AddEditOpenAppModal
        visible={modalVisible}
        app={editingApp}
        onClose={() => setModalVisible(false)}
        onSuccess={() => { setModalVisible(false); onRefresh(); }}
      />
    </>
  );
};

export default OpenAppList;
