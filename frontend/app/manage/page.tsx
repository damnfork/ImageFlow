"use client";

import { useState, useEffect, useRef, useCallback } from "react";

import { motion } from "framer-motion";
import Masonry from "react-masonry-css";
import { api } from "../utils/request";
import { useAuth } from "../contexts/AuthContext";
import OIDCLoginModal from "../components/OIDCLoginModal";
import ImageFilters from "../components/ImageFilters";
import ImageCard from "../components/ImageCard";
import ImageModal from "../components/ImageModal";
import { useTheme } from "../hooks/useTheme";
import {
  ImageFile,
  ImageListResponse,
  StatusMessage,
  ImageFilterState,
} from "../types";
import Header from "../components/Header";
import ToastContainer from "../components/ToastContainer";
import { ImageIcon, Spinner } from "../components/ui/icons";

export default function Manage() {
  useTheme(); 
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [showLoginModal, setShowLoginModal] = useState(false);
  const [images, setImages] = useState<ImageFile[]>([]);
  const [selectedImage, setSelectedImage] = useState<ImageFile | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [status, setStatus] = useState<StatusMessage | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [totalImages, setTotalImages] = useState(0);
  const [filters, setFilters] = useState<ImageFilterState>({
    format: "webp",
    orientation: "all",
    tag: "",
  });
  const [isModalOpen, setIsModalOpen] = useState(false);

  const [isFetchingMore, setIsFetchingMore] = useState(false);
  const observer = useRef<IntersectionObserver | null>(null);
  const lastImageElementRef = useCallback(
    (node: HTMLDivElement | null) => {
      if (isLoading || isFetchingMore) return;
      if (observer.current) observer.current.disconnect();
      observer.current = new IntersectionObserver((entries) => {
        if (entries[0].isIntersecting && hasMore) {
          loadMoreImages();
        }
      });
      if (node) observer.current.observe(node);
    },
    [isLoading, isFetchingMore, hasMore]
  );

  // 当用户认证状态改变时获取图片
  useEffect(() => {
    if (isAuthenticated) {
      fetchImages();
    }
  }, [isAuthenticated]);

  const fetchImages = async () => {
    try {
      setIsLoading(true);
      setImages([]);
      setPage(1);
      const data = await api.get<ImageListResponse>("/api/images", {
        page: "1",
        limit: "24", 
        format: filters.format,
        orientation: filters.orientation,
        tag: filters.tag,
      });

      setImages(data.images);
      setHasMore(data.page < data.totalPages);
      
      if (data.total) {
        setTotalImages(data.total);
      }
      setStatus(null);
    } catch (error) {
      console.error("加载图片列表失败:", error);
      setStatus({
        type: "error",
        message: "加载图片列表失败",
      });
    } finally {
      setIsLoading(false);
    }
  };

  const loadMoreImages = async () => {
    if (!hasMore || isFetchingMore) return;
    
    try {
      setIsFetchingMore(true);
      const nextPage = page + 1;
      
      const data = await api.get<ImageListResponse>("/api/images", {
        page: nextPage.toString(),
        limit: "24", 
        format: filters.format,
        orientation: filters.orientation,
        tag: filters.tag,
      });

      setImages(prevImages => [...prevImages, ...data.images]);
      setPage(nextPage);
      setHasMore(data.page < data.totalPages);
      
    } catch (error) {
      console.error("加载更多图片失败:", error);
      setStatus({
        type: "error",
        message: "加载更多图片失败",
      });
    } finally {
      setIsFetchingMore(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      const image = images.find((img) => img.id === id);
      if (!image) return;

      const response = await api.post<{ success: boolean; message: string }>(
        "/api/delete-image",
        {
          id: image.id,
        }
      );

      if (response.success) {
        await fetchImages();
        setStatus({
          type: "success",
          message: response.message,
        });
      } else {
        setStatus({
          type: "error",
          message: response.message,
        });
      }
    } catch (error) {
      console.error("删除失败:", error);
      setStatus({
        type: "error",
        message: "删除失败",
      });
    }
  };

  useEffect(() => {
    fetchImages();
  }, [filters]);

  const handleFilterChange = (
    format: string,
    orientation: string,
    tag: string
  ) => {
    setFilters({ format, orientation, tag });
  };

  // 如果正在加载认证状态，显示加载界面
  if (authLoading) {
    return (
      <div className="max-w-7xl mx-auto px-6 py-8 flex items-center justify-center min-h-96">
        <div className="text-center">
          <div className="w-12 h-12 animate-spin rounded-full border-4 border-gray-300 border-t-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600 dark:text-gray-400">正在加载...</p>
        </div>
      </div>
    )
  }

  // 如果未认证，显示登录提示
  if (!isAuthenticated) {
    return (
      <div className="max-w-7xl mx-auto px-6 py-8">
        <Header onLoginClick={() => setShowLoginModal(true)} title="图片管理" />
        <div className="text-center py-20">
          <div className="max-w-md mx-auto">
            <div className="w-24 h-24 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-full flex items-center justify-center mx-auto mb-6">
              <ImageIcon className="w-12 h-12 text-white" />
            </div>
            <h2 className="text-2xl font-bold text-gray-900 dark:text-white mb-4">
              请先登录
            </h2>
            <p className="text-gray-600 dark:text-gray-400 mb-8">
              您需要登录后才能查看和管理图片
            </p>
            <button
              onClick={() => setShowLoginModal(true)}
              className="inline-flex items-center px-6 py-3 bg-gradient-to-r from-indigo-500 to-purple-600 text-white font-medium rounded-xl hover:from-indigo-600 hover:to-purple-700 transition-all duration-200"
            >
              立即登录
            </button>
          </div>
        </div>
        <OIDCLoginModal
          isOpen={showLoginModal}
          onClose={() => setShowLoginModal(false)}
          showApiKeyFallback={true}
        />
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-6 py-8">
      <Header
        onLoginClick={() => setShowLoginModal(true)}
        title="图片管理"
      />

      <ToastContainer />

      {status && (
        <motion.div
          initial={{ opacity: 0, y: -20 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -20 }}
          className={`mb-8 p-4 rounded-xl ${
            status.type === "success"
              ? "bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300 border border-green-200 dark:border-green-800"
              : "bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 border border-red-200 dark:border-red-800"
          }`}
        >
          {status.message}
        </motion.div>
      )}

      <ImageFilters onFilterChange={handleFilterChange} />

      {isLoading ? (
        <div className="flex justify-center items-center h-64">
          <Spinner className="h-12 w-12 text-indigo-500" />
        </div>
      ) : (
        <>
          {images.length > 0 ? (
            <>
              <div className="space-y-8">
                <div
                  className={
                    filters.orientation === "all"
                      ? ""
                      : "grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6"
                  }
                >
                  {filters.orientation === "all" ? (
                    <Masonry
                      breakpointCols={{
                        default: 4,
                        1280: 4,
                        1024: 3,
                        768: 2,
                        640: 1,
                      }}
                      className="my-masonry-grid"
                      columnClassName="my-masonry-grid_column"
                    >
                      {images.map((image, index) => (
                        <motion.div
                          key={image.id}
                          initial={{ opacity: 0 }}
                          animate={{ opacity: 1 }}
                          transition={{
                            duration: 0.3,
                            delay: (index % 24) * 0.05,
                          }}
                          ref={index === images.length - 5 ? lastImageElementRef : null}
                        >
                          <ImageCard
                            image={image}
                            onClick={() => {
                              setSelectedImage(image);
                              setIsModalOpen(true);
                            }}
                            onDelete={handleDelete}
                          />
                        </motion.div>
                      ))}
                    </Masonry>
                  ) : (
                    images.map((image, index) => (
                      <motion.div
                        key={image.id}
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        transition={{ duration: 0.3, delay: (index % 24) * 0.05 }}
                        ref={index === images.length - 5 ? lastImageElementRef : null}
                      >
                        <ImageCard
                          image={image}
                          onClick={() => {
                            setSelectedImage(image);
                            setIsModalOpen(true);
                          }}
                          onDelete={handleDelete}
                        />
                      </motion.div>
                    ))
                  )}
                </div>
              </div>
              {isFetchingMore && (
                <div className="flex justify-center items-center py-8">
                  <Spinner className="h-8 w-8 text-indigo-500" />
                  <span className="ml-2 text-indigo-500">加载更多图片...</span>
                </div>
              )}
              {!isLoading && !isFetchingMore && images.length > 0 && !hasMore && (
                <div className="text-center py-8 text-gray-500 dark:text-gray-400">
                  已加载全部图片 ({totalImages}张)
                </div>
              )}
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-64 bg-white dark:bg-slate-800 rounded-xl shadow-md p-8 text-gray-500 dark:text-gray-400 border border-gray-100 dark:border-gray-700">
              <ImageIcon className="w-16 h-16 mb-4 text-gray-300 dark:text-gray-600" />
              <p className="text-lg font-medium">暂无图片</p>
              <p className="mt-2 text-sm">请上传图片或调整筛选条件</p>
            </div>
          )}
        </>
      )}

      <ImageModal
        image={selectedImage}
        isOpen={isModalOpen}
        onClose={() => {
          setSelectedImage(null);
          setIsModalOpen(false);
        }}
        onDelete={handleDelete}
      />

      <OIDCLoginModal
        isOpen={showLoginModal}
        onClose={() => setShowLoginModal(false)}
        showApiKeyFallback={true}
      />
    </div>
  );
}
